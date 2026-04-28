import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { lensesAPI, IdentitySummary } from '../api/client';
import { formatSourceSystem } from '../utils/formatSourceSystem';

type LensTab = 'privileged' | 'cross-cloud-admins' | 'deadends' | 'no-mfa';

export default function LensesPage() {
  const [searchParams] = useSearchParams();
  const [tab, setTab] = useState<LensTab>('privileged');
  const [sourceFilter, setSourceFilter] = useState('');
  const [items, setItems] = useState<IdentitySummary[]>([]);
  const [count, setCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const t = searchParams.get('tab');
    if (t === 'privileged' || t === 'cross-cloud-admins' || t === 'deadends' || t === 'no-mfa') {
      setTab(t);
    }
  }, [searchParams]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    const params: { source_system?: string; limit?: number } = { limit: 100 };
    if (sourceFilter) params.source_system = sourceFilter;

    const fetch = async () => {
      try {
        let res: { items: IdentitySummary[]; count: number };
        if (tab === 'privileged') res = await lensesAPI.privileged(params);
        else if (tab === 'cross-cloud-admins') res = await lensesAPI.crossCloudAdmins({ limit: 100 });
        else if (tab === 'deadends') res = await lensesAPI.deadends({ limit: 100 });
        else res = await lensesAPI.noMfa(params);
        if (!cancelled) {
          setItems(res.items || []);
          setCount(res.count ?? res.items?.length ?? 0);
        }
      } catch (e: any) {
        if (!cancelled) setError(e.message || 'Failed to load');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    fetch();
    return () => { cancelled = true; };
  }, [tab, sourceFilter]);

  const tabs: { id: LensTab; label: string }[] = [
    { id: 'privileged', label: 'Privileged identities' },
    { id: 'cross-cloud-admins', label: 'Cross-system admins' },
    { id: 'deadends', label: 'Deadends' },
    { id: 'no-mfa', label: 'No MFA (privileged)' },
  ];

  const showSourceFilter = tab === 'privileged' || tab === 'no-mfa';

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-50">Cross-system lenses</h1>
        <p className="text-sm text-slate-400 mt-1">
          Discovery lists across all systems. Click an identity to open their dossier.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-4">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-4 py-2 rounded-lg text-sm font-medium ${
              tab === t.id
                ? 'bg-cyan-500/20 text-cyan-200 border border-cyan-400/50'
                : 'bg-slate-800/60 text-slate-300 border border-slate-600/60 hover:bg-slate-700/60'
            }`}
          >
            {t.label}
          </button>
        ))}
        {showSourceFilter && (
          <input
            type="text"
            placeholder="Filter by source system (e.g. okta_mock)"
            value={sourceFilter}
            onChange={(e) => setSourceFilter(e.target.value)}
            className="px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm w-64"
          />
        )}
      </div>

      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl overflow-hidden">
        <div className="px-6 py-4 border-b border-slate-700/60 flex items-center justify-between">
          <span className="text-slate-400 text-sm">
            {loading ? 'Loading…' : `${count} identities`}
          </span>
        </div>
        {error && (
          <div className="px-6 py-4 text-rose-400 text-sm">{error}</div>
        )}
        {!loading && !error && items.length === 0 && (
          <div className="px-6 py-12 text-slate-400 text-center text-sm">No identities in this lens.</div>
        )}
        {!loading && !error && items.length > 0 && (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-slate-800">
              <thead className="bg-slate-900/80">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase">Identity</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase">Systems</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase">Risk</th>
                  {tab === 'deadends' && (
                    <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase">Why</th>
                  )}
                  <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {items.map((row) => (
                  <tr key={row.identity_id} className="hover:bg-slate-800/40">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className="font-medium text-slate-50">{row.display_name || row.email}</span>
                      <div className="text-xs text-slate-400">{row.email}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                      {(row.source_systems || []).map((s) => formatSourceSystem(s)).join(', ')}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm">
                      {row.risk_score != null && (
                        <span className="text-amber-300">{row.risk_score}</span>
                      )}
                      {row.max_severity && (
                        <span className="ml-1 text-slate-400 capitalize">{row.max_severity}</span>
                      )}
                    </td>
                    {tab === 'deadends' && (
                      <td className="px-6 py-4 text-sm text-slate-300 max-w-xs truncate" title={row.deadend_reason || ''}>
                        {row.deadend_reason || '—'}
                      </td>
                    )}
                    <td className="px-6 py-4 whitespace-nowrap">
                      <Link
                        to={`/identities/${row.identity_id}`}
                        className="text-cyan-400 hover:text-cyan-300 text-sm"
                      >
                        Open dossier →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
