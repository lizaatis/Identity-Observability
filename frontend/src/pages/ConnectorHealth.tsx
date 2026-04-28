import { useEffect, useState } from 'react';
import { connectorAPI, ConnectorFullStatusItem } from '../api/client';
import { formatSourceSystem } from '../utils/formatSourceSystem';

export default function ConnectorHealth() {
  const [connectors, setConnectors] = useState<ConnectorFullStatusItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadData = async () => {
      try {
        const list = await connectorAPI.listFullStatus();
        if (list.length === 0) {
          setError(
            'No connector sync runs found. Run migrations, then run the mock or real connector so sync_runs is populated.'
          );
          setConnectors([]);
        } else {
          setConnectors(list);
        }
      } catch (err) {
        console.error('Failed to load connectors:', err);
        setError('Failed to load connector data. Make sure the backend is running and connectors have been synced.');
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, []);

  return (
    <div className="space-y-6">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <h1 className="text-2xl font-semibold text-slate-50">Connector Health</h1>
        <p className="mt-1 text-sm text-slate-400">
          Status of each data connector, last sync, and recent errors.
        </p>
      </div>

      {loading && (
        <div className="flex items-center justify-center min-h-[200px]">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-cyan-400" />
            <p className="mt-3 text-sm text-slate-300">Loading connector health…</p>
          </div>
        </div>
      )}

      {!loading && error && (
        <div className="bg-amber-900/40 border border-amber-500/60 rounded-2xl p-4 text-sm text-amber-100">
          {error}
        </div>
      )}

      {!loading && !error && connectors.length === 0 && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 text-sm text-slate-300">
          No connectors found. Run a connector sync to see status information.
        </div>
      )}

      {!loading && !error && connectors.map((connector) => (
        <div
          key={connector.connector_id}
          className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]"
        >
          <div className="flex justify-between items-start mb-6">
            <div>
              <h2 className="text-lg font-semibold text-slate-50">{connector.connector_name}</h2>
              <p className="text-xs text-slate-400 mt-1">
                Source System: {formatSourceSystem(connector.source_system)}
              </p>
            </div>
            <span
              className={`px-4 py-2 rounded-lg font-medium ${
                connector.status === 'healthy'
                  ? 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/50'
                  : connector.status === 'degraded'
                  ? 'bg-amber-500/20 text-amber-200 border border-amber-400/50'
                  : 'bg-rose-500/20 text-rose-200 border border-rose-400/50'
              }`}
            >
              {connector.status.toUpperCase()}
            </span>
          </div>

          {connector.last_sync && (
            <div className="mb-6">
              <h3 className="text-sm font-semibold text-slate-100 mb-3">Last Sync</h3>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div>
                  <p className="text-xs text-slate-400">Started</p>
                  <p className="font-medium text-slate-100">
                    {new Date(connector.last_sync.started_at).toLocaleString()}
                  </p>
                </div>
                {connector.last_sync.finished_at && (
                  <div>
                    <p className="text-xs text-slate-400">Finished</p>
                    <p className="font-medium text-slate-100">
                      {new Date(connector.last_sync.finished_at).toLocaleString()}
                    </p>
                  </div>
                )}
                {connector.last_sync.duration && (
                  <div>
                    <p className="text-xs text-slate-400">Duration</p>
                    <p className="font-medium text-slate-100">{connector.last_sync.duration}</p>
                  </div>
                )}
                <div>
                  <p className="text-xs text-slate-400">Status</p>
                  <p
                    className={`font-medium capitalize ${
                      connector.last_sync.status === 'success'
                        ? 'text-emerald-300'
                        : connector.last_sync.status === 'partial'
                        ? 'text-amber-300'
                        : 'text-rose-300'
                    }`}
                  >
                    {connector.last_sync.status}
                  </p>
                </div>
              </div>
              <div className="mt-4 grid grid-cols-2 gap-4">
                <div>
                  <p className="text-xs text-slate-400">Errors</p>
                  <p className={`text-2xl font-bold ${connector.last_sync.error_count > 0 ? 'text-rose-400' : 'text-slate-100'}`}>
                    {connector.last_sync.error_count}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-slate-400">Warnings</p>
                  <p className={`text-2xl font-bold ${connector.last_sync.warning_count > 0 ? 'text-amber-300' : 'text-slate-100'}`}>
                    {connector.last_sync.warning_count}
                  </p>
                </div>
              </div>
              {connector.last_sync.last_error && (
                <div className="mt-4 p-4 bg-rose-950/60 border border-rose-500/60 rounded-lg">
                  <p className="text-sm font-medium text-rose-200">Last Error</p>
                  <p className="text-xs text-rose-200/80 mt-1">{connector.last_sync.last_error}</p>
                </div>
              )}
              {connector.row_counts && Object.keys(connector.row_counts).length > 0 && (
                <div className="mt-4">
                  <p className="text-xs text-slate-400 mb-2">Row counts (from last sync metadata)</p>
                  <div className="flex flex-wrap gap-2">
                    {Object.entries(connector.row_counts).map(([k, v]) => (
                      <span
                        key={k}
                        className="px-2 py-1 rounded-md text-xs bg-slate-800/80 border border-slate-600/50 text-slate-200"
                      >
                        {k}: {String(v)}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {connector.recent_syncs && connector.recent_syncs.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-slate-100 mb-3">Recent Syncs</h3>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-slate-800">
                  <thead className="bg-slate-900/80">
                    <tr>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Started</th>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Finished</th>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Status</th>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Errors</th>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Warnings</th>
                      <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase">Duration</th>
                    </tr>
                  </thead>
                  <tbody className="bg-slate-950 divide-y divide-slate-800">
                    {connector.recent_syncs.map((sync) => (
                      <tr key={sync.id}>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-100">
                          {new Date(sync.started_at).toLocaleString()}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-400">
                          {sync.finished_at ? new Date(sync.finished_at).toLocaleString() : '-'}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <span
                            className={`px-2 py-1 text-xs font-medium rounded capitalize ${
                              sync.status === 'success'
                                ? 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/50'
                                : sync.status === 'partial'
                                ? 'bg-amber-500/20 text-amber-200 border border-amber-400/50'
                                : 'bg-rose-500/20 text-rose-200 border border-rose-400/50'
                            }`}
                          >
                            {sync.status}
                          </span>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-200">
                          {sync.error_count}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-200">
                          {sync.warning_count}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-400">
                          {sync.duration || '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
