import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { identityAPI, IdentityDetail, RiskScore, changesAPI, ChangeItem } from '../api/client';
import RiskBadge from '../components/RiskBadge';
import ExportButton from '../components/ExportButton';
import Timeline from '../components/Timeline';
import BlastRadius from '../components/BlastRadius';
import RemediationAction from '../components/RemediationAction';
import { formatSourceSystem } from '../utils/formatSourceSystem';

export default function IdentityProfile() {
  const { id } = useParams<{ id: string }>();
  const [identity, setIdentity] = useState<IdentityDetail | null>(null);
  const [risk, setRisk] = useState<RiskScore | null>(null);
  const [changes, setChanges] = useState<ChangeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;

    const loadData = async () => {
      try {
        setLoading(true);
        const [identityData, riskData, changesRes] = await Promise.all([
          identityAPI.get(Number(id)),
          identityAPI.getRisk(Number(id)),
          changesAPI.get({ identity: id }).catch(() => ({ changes: [] })),
        ]);
        setIdentity(identityData);
        setRisk(riskData);
        setChanges(changesRes.changes || []);
      } catch (err: any) {
        setError(err.message || 'Failed to load identity');
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, [id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[300px]">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-10 w-10 border-b-2 border-cyan-400"></div>
          <p className="mt-4 text-slate-300 text-sm">Loading identity telemetry…</p>
        </div>
      </div>
    );
  }

  if (error || !identity) {
    return (
      <div className="text-center py-12 text-rose-400">
        Error: {error || 'Identity not found'}
      </div>
    );
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <div className="flex justify-between items-start">
          <div>
            <p className="text-[11px] font-semibold text-cyan-300 uppercase tracking-[0.25em]">
              Identity Flight Record
            </p>
            <h1 className="mt-2 text-3xl font-semibold text-slate-50">
              {identity.identity.display_name || identity.identity.email}
            </h1>
            <p className="mt-1 text-sm text-slate-400">{identity.identity.email}</p>
            {identity.identity.employee_id && (
              <p className="text-xs text-slate-500 mt-1">Employee ID: {identity.identity.employee_id}</p>
            )}
          </div>
          <div className="flex items-center space-x-4">
            {risk && <RiskBadge score={risk.score} severity={risk.max_severity} />}
            <ExportButton identityId={Number(id)} />
            {risk && risk.flags.length > 0 && (
              <RemediationAction
                identityId={Number(id)}
                riskType={risk.flags[0]?.rule_key}
              />
            )}
          </div>
        </div>
      </div>

      {identity.stitching_summary && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-slate-50">Same person across systems</h2>
              <p className="mt-1 text-xs text-slate-500 uppercase tracking-wider">
                Stitching confidence · {identity.stitching_summary.confidence.replace(/_/g, ' ')}
              </p>
            </div>
            {identity.stitching_summary.needs_review && (
              <span className="px-3 py-1 rounded-full text-xs font-medium bg-amber-500/20 text-amber-100 border border-amber-400/50">
                Review suggested
              </span>
            )}
          </div>
          <ul className="mt-4 space-y-2 text-sm text-slate-300 list-disc list-inside">
            {identity.stitching_summary.reasons.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-slate-500">
            Linked sources: {identity.stitching_summary.source_count}. Ambiguous cases can be queued for review (
            <code className="text-cyan-200/80">GET /api/v1/stitching/review</code>) before ML stitching.
          </p>
        </div>
      )}

      {/* Recent changes for this identity */}
      {changes.length > 0 && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6">
          <h2 className="text-lg font-semibold text-slate-50 mb-3">Recent changes</h2>
          <ul className="space-y-2">
            {changes.slice(0, 10).map((ch) => (
              <li key={ch.id} className="text-sm text-slate-300 flex items-center gap-2">
                <span className="text-slate-500 text-xs">{new Date(ch.event_time).toLocaleString()}</span>
                <span>{ch.summary}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Compact summary strip: "Okta: Super Admin (MFA: Yes)" etc. */}
      <div className="flex flex-wrap items-center gap-2 p-4 rounded-xl bg-slate-900/60 border border-slate-700/60">
        <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider mr-1">Systems</span>
        {(identity.system_summary || []).map((sys) => (
          <span
            key={sys.system}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm border bg-slate-800/80 border-slate-600/60 text-slate-200"
          >
            <span className="font-medium text-cyan-200">{formatSourceSystem(sys.system)}</span>
            {sys.is_admin && (
              <span className="px-1.5 py-0.5 rounded text-xs font-medium bg-amber-500/20 text-amber-200 border border-amber-400/40">
                {sys.admin_roles.length ? sys.admin_roles[0] : 'Admin'}
              </span>
            )}
            <span className="text-slate-400 text-xs">
              MFA: {sys.mfa_enabled === true ? 'Yes' : sys.mfa_enabled === false ? 'No' : '—'}
            </span>
            <span className="text-slate-500 text-[10px]">
              synced {new Date(sys.last_sync_at).toLocaleDateString()}
            </span>
            <span
              className={`text-[10px] px-1.5 py-0.5 rounded font-medium border ${freshnessBadgeClass(sys.data_freshness)}`}
              title="Data freshness from last connector sync time"
            >
              {freshnessLabel(sys.data_freshness)}
            </span>
          </span>
        ))}
        {(identity.deadend_summary?.count ?? 0) > 0 && (
          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm border bg-rose-500/15 border-rose-400/40 text-rose-200">
            <span className="font-semibold">Deadends: {identity.deadend_summary!.count}</span>
            <span className="text-rose-300/90 text-xs max-w-[200px] truncate" title={identity.deadend_summary!.reasons[0] || ''}>
              ({identity.deadend_summary!.reasons[0] ? identity.deadend_summary!.reasons[0].slice(0, 35).trim() + (identity.deadend_summary!.reasons[0].length > 35 ? '…' : '') : 'see below'})
            </span>
          </span>
        )}
      </div>

      {/* Status per Source System */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-slate-50">Source Systems</h2>
          <span className="text-xs text-slate-500">
            {identity.sources.length} linked accounts
          </span>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {identity.sources.map((source) => (
            <div
              key={`${source.source_system}-${source.source_user_id}`}
              className="rounded-xl p-4 border border-slate-800/80 bg-slate-900/80"
            >
              <div className="flex justify-between items-start">
                <div>
                  <h3 className="font-medium text-slate-50">{formatSourceSystem(source.source_system)}</h3>
                  <p className="text-xs text-slate-400 mt-1">ID: {source.source_user_id}</p>
                </div>
                <span
                  className={`px-2 py-1 text-xs font-medium rounded ${
                    source.source_status === 'active'
                      ? 'bg-emerald-500/15 text-emerald-300 border border-emerald-400/40'
                      : 'bg-rose-500/15 text-rose-300 border border-rose-400/40'
                  }`}
                >
                  {source.source_status}
                </span>
              </div>
              <p className="text-[11px] text-slate-500 mt-2 flex flex-wrap items-center gap-2">
                <span>Last sync: {new Date(source.synced_at).toLocaleString()}</span>
                <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium border ${freshnessBadgeClass(source.data_freshness)}`}>
                  {freshnessLabel(source.data_freshness)}
                </span>
              </p>
            </div>
          ))}
        </div>
      </div>

      {/* Risk Score + Flags */}
      {risk && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-lg font-semibold text-slate-50">Risk Assessment</h2>
            <button
              onClick={async () => {
                const newRisk = await identityAPI.getRisk(Number(id), true);
                setRisk(newRisk);
              }}
              className="text-xs text-cyan-300 hover:text-cyan-200"
            >
              Recompute
            </button>
          </div>
          <div className="mb-6">
            <div className="flex items-center space-x-4">
              <div>
                <span className="text-xs text-slate-400">Risk Score</span>
                <div className="text-4xl font-bold" style={{ color: getRiskColor(risk.score) }}>
                  {risk.score}
                </div>
              </div>
              <div>
                <span className="text-xs text-slate-400">Max Severity</span>
                <div className="text-2xl font-semibold capitalize text-slate-50">
                  {risk.max_severity}
                </div>
              </div>
            </div>
          </div>
          <div>
            <h3 className="font-medium mb-3 text-slate-50">Risk Flags ({risk.flags.length})</h3>
            <div className="space-y-2">
              {risk.flags.map((flag, idx) => (
                <div
                  key={idx}
                  className={`border-l-4 p-3 rounded-xl ${
                    flag.is_deadend
                      ? 'border-rose-500 bg-rose-500/10'
                      : 'border-amber-400 bg-amber-500/5'
                  }`}
                >
                  <div className="flex justify-between items-start">
                    <div>
                      <div className="flex items-center space-x-2">
                        <span className="font-medium text-slate-50">{flag.message}</span>
                        {flag.is_deadend && (
                          <span className="px-2 py-0.5 text-xs font-medium bg-red-100 text-red-800 rounded">
                            DEADEND
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-slate-300 mt-1">Rule: {flag.rule_key}</p>
                      <p className="text-[11px] text-slate-400 mt-1">Severity: {flag.severity}</p>
                    </div>
                    <span
                      className={`px-2 py-1 text-xs font-medium rounded capitalize ${
                        flag.severity === 'critical'
                          ? 'bg-rose-600/20 text-rose-200 border border-rose-400/50'
                          : flag.severity === 'high'
                          ? 'bg-orange-500/20 text-orange-200 border border-orange-400/50'
                          : 'bg-amber-500/20 text-amber-100 border border-amber-400/40'
                      }`}
                    >
                      {flag.severity}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Effective Permissions */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-semibold text-slate-50">
            Effective Permissions ({identity.effective_permissions.length})
          </h2>
          <Link
            to={`/identities/${id}/explain`}
            className="text-xs text-cyan-300 hover:text-cyan-200"
          >
            View Lineage →
          </Link>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-800">
            <thead className="bg-slate-900/80">
              <tr>
                <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider">
                  Permission
                </th>
                <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider">
                  Resource Type
                </th>
                <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider">
                  Role
                </th>
                <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider">
                  Path
                </th>
                <th className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider">
                  Group
                </th>
              </tr>
            </thead>
            <tbody className="bg-slate-950 divide-y divide-slate-800">
              {identity.effective_permissions.map((perm, idx) => (
                <tr key={idx}>
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-slate-50">
                    {perm.permission_name}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                    {perm.resource_type || '-'}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                    {perm.role_name} ({perm.privilege_level})
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                    <span
                      className={`px-2 py-1 text-xs rounded ${
                        perm.path_type === 'direct_role'
                          ? 'bg-cyan-500/20 text-cyan-200 border border-cyan-400/40'
                          : 'bg-purple-500/20 text-purple-200 border border-purple-400/40'
                      }`}
                    >
                      {perm.path_type === 'direct_role' ? 'Direct Role' : 'Via Group'}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                    {perm.group_name || '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Timeline */}
      <Timeline identityId={Number(id)} />

      {/* Blast Radius */}
      <BlastRadius identityId={Number(id)} />
    </div>
  );
}

function getRiskColor(score: number): string {
  if (score >= 70) return '#dc2626'; // red
  if (score >= 40) return '#ea580c'; // orange
  if (score >= 20) return '#f59e0b'; // yellow
  return '#10b981'; // green
}

function freshnessLabel(f?: string): string {
  switch (f) {
    case 'fresh':
      return 'Fresh';
    case 'stale':
      return 'Stale';
    case 'very_stale':
      return 'Very stale';
    default:
      return 'Unknown';
  }
}

function freshnessBadgeClass(f?: string): string {
  switch (f) {
    case 'fresh':
      return 'bg-emerald-500/15 text-emerald-200 border-emerald-400/40';
    case 'stale':
      return 'bg-amber-500/15 text-amber-100 border-amber-400/40';
    case 'very_stale':
      return 'bg-rose-500/15 text-rose-100 border-rose-400/40';
    default:
      return 'bg-slate-700/50 text-slate-300 border-slate-500/40';
  }
}
