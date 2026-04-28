import { useEffect, useState } from 'react';
import { graphAPI, ToxicComboMatch, GraphStatusResponse } from '../api/client';

export default function ToxicComboPage() {
  const [matches, setMatches] = useState<ToxicComboMatch[]>([]);
  const [graphStatus, setGraphStatus] = useState<GraphStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadMatches = async () => {
      try {
        setLoading(true);
        const [results, gs] = await Promise.all([
          graphAPI.getToxicCombo(),
          graphAPI.getGraphStatus().catch(() => null),
        ]);
        setMatches(results);
        setGraphStatus(gs);
      } catch (error) {
        console.error('Failed to load toxic combo matches:', error);
      } finally {
        setLoading(false);
      }
    };

    loadMatches();
  }, []);

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical':
        return 'bg-red-100 text-red-800 border-red-300';
      case 'high':
        return 'bg-orange-100 text-orange-800 border-orange-300';
      case 'medium':
        return 'bg-yellow-100 text-yellow-800 border-yellow-300';
      default:
        return 'bg-gray-100 text-gray-800 border-gray-300';
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[300px]">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-10 w-10 border-b-2 border-cyan-400"></div>
          <p className="mt-4 text-slate-300 text-sm">Loading toxic combo matches…</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <h1 className="text-2xl font-semibold text-slate-50 mb-1">Toxic Combo Rules</h1>
        <p className="text-sm text-slate-400">
          Security patterns detected in the identity graph (Neo4j paths; Postgres remains source of truth).
        </p>
        {graphStatus && (
          <p className="mt-3 text-xs text-slate-500">
            Graph: Neo4j {graphStatus.neo4j_reachable ? 'reachable' : 'not reachable'}
            {graphStatus.last_sync_at && (
              <>
                {' '}
                · last sync {new Date(graphStatus.last_sync_at).toLocaleString()}
                {graphStatus.node_count != null && graphStatus.edge_count != null && (
                  <span>
                    {' '}
                    · {graphStatus.node_count} nodes, {graphStatus.edge_count} edges
                  </span>
                )}
              </>
            )}
            {graphStatus.last_error && <span className="text-rose-300/90"> · {graphStatus.last_error}</span>}
          </p>
        )}
      </div>

      {matches.length === 0 ? (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 text-center">
          <p className="text-sm text-slate-300">
            No toxic combo matches found. If Neo4j is new or empty, run <strong className="text-slate-200">Sync Graph</strong>{' '}
            (<code className="text-cyan-200/80">POST /api/v1/graph/sync</code>) after Postgres has identities and relationships.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {matches.map((match, idx) => (
            <div
              key={idx}
              className={`rounded-2xl p-6 border-l-4 shadow-[0_0_24px_rgba(15,23,42,0.9)] bg-slate-950/90 ${getSeverityColor(
                match.severity
              )}`}
            >
              <div className="flex justify-between items-start mb-4">
                <div>
                  <h2 className="text-lg font-semibold text-slate-50">{match.rule_name}</h2>
                  <p className="text-sm text-slate-400 mt-1">
                    {match.count} match{match.count !== 1 ? 'es' : ''} found
                  </p>
                </div>
                <span
                  className={`px-3 py-1 rounded-full text-xs font-semibold uppercase tracking-wide ${getSeverityColor(
                    match.severity
                  )}`}
                >
                  {match.severity}
                </span>
              </div>

              {match.matches.length > 0 && (
                <div className="mt-4 overflow-x-auto">
                  <table className="min-w-full divide-y divide-slate-800">
                    <thead className="bg-slate-900/80">
                      <tr>
                        {Object.keys(match.matches[0]).map((key) => (
                          <th
                            key={key}
                            className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider"
                          >
                            {key.replace(/_/g, ' ')}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="bg-slate-950 divide-y divide-slate-800">
                      {match.matches.slice(0, 10).map((m, mIdx) => (
                        <tr key={mIdx}>
                          {Object.values(m).map((value, vIdx) => (
                            <td
                              key={vIdx}
                              className="px-6 py-4 whitespace-nowrap text-sm text-slate-200"
                            >
                              {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {match.matches.length > 10 && (
                    <p className="text-sm text-slate-400 mt-2 text-center">
                      Showing 10 of {match.matches.length} matches
                    </p>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
