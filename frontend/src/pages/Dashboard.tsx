import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, PieChart, Pie, Cell, ResponsiveContainer } from 'recharts';
import { 
  ShieldExclamationIcon, 
  UserGroupIcon, 
  LockClosedIcon, 
  ExclamationTriangleIcon,
  ArrowDownTrayIcon,
  ChartBarIcon
} from '@heroicons/react/24/outline';
import {
  riskAPI,
  TopRiskIdentity,
  exportAPI,
  dashboardAPI,
  DashboardStats,
  changesAPI,
  ChangeItem,
  platformAPI,
  PlatformDiagnostics,
} from '../api/client';
import RiskBadge from '../components/RiskBadge';
import { formatSourceSystem } from '../utils/formatSourceSystem';

export default function Dashboard() {
  const [topRisks, setTopRisks] = useState<TopRiskIdentity[]>([]);
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [showNoMfa, setShowNoMfa] = useState(false);
  const [showDeadends, setShowDeadends] = useState(false);
  const [showCrossCloud, setShowCrossCloud] = useState(false);
  const [showPrivileged, setShowPrivileged] = useState(false);
  const [changes, setChanges] = useState<ChangeItem[]>([]);
  const [diag, setDiag] = useState<PlatformDiagnostics | null>(null);

  useEffect(() => {
    const loadData = async () => {
      try {
        const [riskData, dashboardStats, changesRes, diagRes] = await Promise.all([
          riskAPI.getTop(20),
          dashboardAPI.getStats(),
          changesAPI.get({ since: '24h' }).catch(() => ({ changes: [] })),
          platformAPI.getDiagnostics().catch(() => null),
        ]);
        setTopRisks(riskData.identities || []);
        setStats(dashboardStats);
        setChanges(changesRes.changes || []);
        setDiag(diagRes);
      } catch (error) {
        console.error('Failed to load dashboard data:', error);
      } finally {
        setLoading(false);
      }
    };

    // Initial load
    loadData();

    // Real-time updates: poll every 30 seconds
    const interval = setInterval(() => {
      loadData();
    }, 30000); // 30 seconds

    return () => clearInterval(interval);
  }, []);

  const handleExport = async (format: 'csv' | 'pdf', type: 'high-risk' | 'deadends') => {
    try {
      const blob = type === 'high-risk' 
        ? await exportAPI.exportHighRisk(format)
        : await exportAPI.exportDeadends(format);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${type}-${new Date().toISOString()}.${format}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error) {
      alert('Export failed');
    }
  };

  // Severity distribution from backend stats (all identities) or fallback to topRisks
  const severityDistribution = stats?.risk_distribution
    ? [
        { name: 'Critical', value: stats.risk_distribution.critical || 0, color: '#dc2626' },
        { name: 'High', value: stats.risk_distribution.high || 0, color: '#ea580c' },
        { name: 'Medium', value: stats.risk_distribution.medium || 0, color: '#f59e0b' },
        { name: 'Low', value: stats.risk_distribution.low || 0, color: '#10b981' },
      ]
    : [
        { name: 'Critical', value: topRisks.filter((i) => i.max_severity === 'critical').length, color: '#dc2626' },
        { name: 'High', value: topRisks.filter((i) => i.max_severity === 'high').length, color: '#ea580c' },
        { name: 'Medium', value: topRisks.filter((i) => i.max_severity === 'medium').length, color: '#f59e0b' },
        { name: 'Low', value: topRisks.filter((i) => i.max_severity === 'low').length, color: '#10b981' },
      ];

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-cyan-400"></div>
          <p className="mt-4 text-slate-300">Loading dashboard telemetry…</p>
        </div>
      </div>
    );
  }

  const showDiagBanner =
    diag &&
    (!diag.database_ok ||
      diag.messages.length > 0 ||
      (diag.neo4j_configured && !diag.neo4j_reachable) ||
      (diag.database_ok && (stats?.total_identities ?? 0) === 0 && diag.identity_count === 0));

  return (
    <div className="space-y-8">
      {showDiagBanner && diag && (
        <div className="rounded-2xl border border-amber-500/40 bg-amber-950/30 px-5 py-4 text-sm text-amber-100/95">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="font-semibold text-amber-50">Environment &amp; data checks</p>
              {!diag.database_ok && (
                <p className="mt-1 text-amber-200/90">Database is not reachable — API cannot load real data.</p>
              )}
              {stats?.total_identities === 0 && diag.identity_count === 0 && diag.database_ok && (
                <p className="mt-1 text-amber-200/90">
                  No identities in Postgres — run migrations and a connector sync (see Setup runbook).
                </p>
              )}
              {diag.messages.map((m, i) => (
                <p key={i} className="mt-1 text-amber-100/85 text-xs">
                  {m}
                </p>
              ))}
            </div>
            <Link
              to="/setup"
              className="shrink-0 rounded-full border border-amber-400/50 px-3 py-1.5 text-xs font-medium text-amber-50 hover:bg-amber-500/15"
            >
              Open setup runbook
            </Link>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <h1 className="text-3xl font-semibold text-slate-50 tracking-tight">Risk Radar</h1>
          <p className="mt-2 text-sm text-slate-400">
            Live view of privileged access, MFA posture, and deadends across your identity graph.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            onClick={() => handleExport('csv', 'high-risk')}
            className="inline-flex items-center px-4 py-2 rounded-full border border-cyan-400/40 bg-slate-900/70 text-cyan-100 hover:bg-cyan-500/20 hover:border-cyan-300/70 shadow-[0_0_0_1px_rgba(34,211,238,0.3)] transition-all duration-200 text-xs font-semibold tracking-wide"
          >
            <ArrowDownTrayIcon className="h-4 w-4 mr-2 text-cyan-300" />
            Export High Risk (CSV)
          </button>
          <button
            onClick={() => handleExport('pdf', 'high-risk')}
            className="inline-flex items-center px-4 py-2 rounded-full border border-cyan-400/20 bg-slate-900/60 text-slate-200 hover:bg-cyan-500/10 hover:border-cyan-400/60 transition-all duration-200 text-xs font-semibold tracking-wide"
          >
            <ArrowDownTrayIcon className="h-4 w-4 mr-2 text-cyan-200" />
            Export High Risk (PDF)
          </button>
          <button
            onClick={() => handleExport('csv', 'deadends')}
            className="inline-flex items-center px-4 py-2 rounded-full border border-rose-500/40 bg-slate-900/70 text-rose-100 hover:bg-rose-500/15 hover:border-rose-400/70 shadow-[0_0_0_1px_rgba(244,63,94,0.4)] transition-all duration-200 text-xs font-semibold tracking-wide"
          >
            <ArrowDownTrayIcon className="h-4 w-4 mr-2 text-rose-300" />
            Export Deadends
          </button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
        <button
          onClick={() => {
            setShowPrivileged(!showPrivileged);
            if (!showPrivileged && stats?.privileged_identities && stats.privileged_identities.length > 0) {
              setTimeout(() => {
                document.getElementById('privileged-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }, 100);
            }
          }}
          className="group relative overflow-hidden rounded-2xl p-5 text-left w-full bg-gradient-to-br from-slate-900/90 via-slate-900/70 to-slate-950 border border-cyan-500/30 shadow-[0_0_25px_rgba(8,47,73,0.7)] hover:shadow-[0_0_35px_rgba(34,211,238,0.7)] transition-all cursor-pointer"
        >
          <div className="absolute inset-px rounded-2xl bg-[radial-gradient(circle_at_top,_rgba(34,211,238,0.18),_transparent_60%)] pointer-events-none" />
          <div className="relative flex items-center justify-between">
            <div>
              <p className="text-[11px] font-semibold text-cyan-300 uppercase tracking-[0.25em]">
                Privileged Identities
              </p>
              <p className="mt-3 text-4xl font-semibold text-slate-50">
                {stats?.privileged_count || 0}
              </p>
              <p className="mt-1 text-[11px] text-cyan-200/80">
                {stats?.privileged_identities && stats.privileged_identities.length > 0
                  ? `${stats.privileged_identities.length} identities - Click to view`
                  : 'Identities with elevated access'}
              </p>
            </div>
            <div className="p-3 rounded-xl bg-slate-900/80 border border-cyan-400/40 shadow-inner shadow-cyan-500/40">
              <ShieldExclamationIcon className="h-7 w-7 text-cyan-300" />
            </div>
          </div>
        </button>

        <button
          onClick={() => {
            setShowCrossCloud(!showCrossCloud);
            if (!showCrossCloud && stats?.cross_cloud_admin_list && stats.cross_cloud_admin_list.length > 0) {
              setTimeout(() => {
                document.getElementById('cross-cloud-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }, 100);
            }
          }}
          className="group relative overflow-hidden rounded-2xl p-5 text-left w-full bg-gradient-to-br from-slate-900/90 via-slate-900/70 to-slate-950 border border-amber-400/30 shadow-[0_0_25px_rgba(120,53,15,0.7)] hover:shadow-[0_0_35px_rgba(251,191,36,0.6)] transition-all cursor-pointer"
        >
          <div className="absolute inset-px rounded-2xl bg-[radial-gradient(circle_at_top,_rgba(251,191,36,0.22),_transparent_60%)] pointer-events-none" />
          <div className="relative flex items-center justify-between">
            <div>
              <p className="text-[11px] font-semibold text-amber-300 uppercase tracking-[0.25em]">
                Cross-Cloud Admins
              </p>
              <p className="mt-3 text-4xl font-semibold text-slate-50">
                {stats?.cross_cloud_admins || 0}
              </p>
              <p className="mt-1 text-[11px] text-amber-200/90">
                {stats?.cross_cloud_admin_list && stats.cross_cloud_admin_list.length > 0
                  ? `${stats.cross_cloud_admin_list.length} identities - Click to view`
                  : 'Admin access in multiple systems'}
              </p>
            </div>
            <div className="p-3 rounded-xl bg-slate-900/80 border border-amber-400/40 shadow-inner shadow-amber-400/50">
              <UserGroupIcon className="h-7 w-7 text-amber-300" />
            </div>
          </div>
        </button>

        <button
          onClick={() => {
            setShowNoMfa(!showNoMfa);
            if (!showNoMfa && stats?.no_mfa_identities && stats.no_mfa_identities.length > 0) {
              setTimeout(() => {
                document.getElementById('no-mfa-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }, 100);
            }
          }}
          className="group relative overflow-hidden rounded-2xl p-5 text-left w-full bg-gradient-to-br from-slate-900/90 via-slate-900/70 to-slate-950 border border-emerald-400/30 shadow-[0_0_25px_rgba(6,95,70,0.7)] hover:shadow-[0_0_35px_rgba(16,185,129,0.7)] transition-all cursor-pointer"
        >
          <div className="absolute inset-px rounded-2xl bg-[radial-gradient(circle_at_top,_rgba(16,185,129,0.22),_transparent_60%)] pointer-events-none" />
          <div className="relative flex items-center justify-between">
            <div>
              <p className="text-[11px] font-semibold text-emerald-300 uppercase tracking-[0.25em]">
                MFA Coverage
              </p>
              <p className="mt-3 text-4xl font-semibold text-slate-50">
                {stats?.mfa_coverage ? Math.round(stats.mfa_coverage) : 0}%
              </p>
              <p className="mt-1 text-[11px] text-emerald-200/90">
                {stats?.no_mfa_identities && stats.no_mfa_identities.length > 0 
                  ? `${stats.no_mfa_identities.length} privileged without MFA - Click to view`
                  : 'Identities with MFA enabled'}
              </p>
            </div>
            <div className="p-3 rounded-xl bg-slate-900/80 border border-emerald-400/40 shadow-inner shadow-emerald-400/50">
              <LockClosedIcon className="h-7 w-7 text-emerald-300" />
            </div>
          </div>
        </button>

        <button
          onClick={() => {
            setShowDeadends(!showDeadends);
            if (!showDeadends && stats?.deadend_identities && stats.deadend_identities.length > 0) {
              setTimeout(() => {
                document.getElementById('deadend-section')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }, 100);
            }
          }}
          className="group relative overflow-hidden rounded-2xl p-5 text-left w-full bg-gradient-to-br from-slate-900/90 via-slate-900/70 to-slate-950 border border-rose-500/35 shadow-[0_0_30px_rgba(220,38,38,0.7)] hover:shadow-[0_0_40px_rgba(248,113,113,0.9)] transition-all cursor-pointer"
        >
          <div className="absolute inset-px rounded-2xl bg-[radial-gradient(circle_at_top,_rgba(248,113,113,0.22),_transparent_60%)] pointer-events-none" />
          <div className="relative flex items-center justify-between">
            <div>
              <p className="text-[11px] font-semibold text-rose-300 uppercase tracking-[0.25em]">
                Deadend Count
              </p>
              <p className="mt-3 text-4xl font-semibold text-slate-50">
                {stats?.deadend_count || 0}
              </p>
              <p className="mt-1 text-[11px] text-rose-200/90">
                {stats?.deadend_identities && stats.deadend_identities.length > 0
                  ? `${stats.deadend_identities.length} identities - Click to view`
                  : 'Critical security issues detected'}
              </p>
            </div>
            <div className="p-3 rounded-xl bg-slate-900/80 border border-rose-500/40 shadow-inner shadow-rose-500/60">
              <ExclamationTriangleIcon className="h-7 w-7 text-rose-300" />
            </div>
          </div>
        </button>
      </div>

      <div className="flex flex-wrap gap-2">
        <Link
          to="/lenses?tab=privileged"
          className="inline-flex items-center px-3 py-1.5 rounded-full text-xs font-medium border border-cyan-400/40 text-cyan-200 hover:bg-cyan-500/15"
        >
          View privileged identities →
        </Link>
        <Link
          to="/lenses?tab=cross-cloud-admins"
          className="inline-flex items-center px-3 py-1.5 rounded-full text-xs font-medium border border-amber-400/40 text-amber-200 hover:bg-amber-500/15"
        >
          View cross-cloud admins →
        </Link>
        <Link
          to="/lenses?tab=no-mfa"
          className="inline-flex items-center px-3 py-1.5 rounded-full text-xs font-medium border border-emerald-400/40 text-emerald-200 hover:bg-emerald-500/15"
        >
          View no-MFA identities →
        </Link>
        <Link
          to="/lenses?tab=deadends"
          className="inline-flex items-center px-3 py-1.5 rounded-full text-xs font-medium border border-rose-400/40 text-rose-200 hover:bg-rose-500/15"
        >
          View deadends →
        </Link>
      </div>

      {/* Last 24h identity changes */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <h2 className="text-lg font-semibold text-slate-50 mb-4">Last 24h identity changes</h2>
        {changes.length === 0 ? (
          <p className="text-sm text-slate-400">
            {diag && !diag.identity_events_table
              ? 'No drift feed: the identity_events table is missing (run migration 006) or no events have been recorded yet.'
              : 'No recent changes in this window. Sync connectors and ingest events (webhooks/realtime) to see drift here.'}
          </p>
        ) : (
          <ul className="space-y-2 max-h-48 overflow-y-auto">
            {changes.slice(0, 15).map((ch) => (
              <li key={ch.id} className="flex items-center justify-between text-sm py-1.5 border-b border-slate-800/60 last:border-0">
                <span className="text-slate-300 truncate flex-1">{ch.summary}</span>
                {ch.identity_id && (
                  <Link to={`/identities/${ch.identity_id}`} className="ml-2 text-cyan-400 hover:text-cyan-300 shrink-0">View →</Link>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h2 className="text-xl font-semibold text-slate-50">Risk Score Distribution</h2>
              <p className="text-sm text-slate-400 mt-1">Top 10 risky identities</p>
            </div>
            <ChartBarIcon className="h-6 w-6 text-cyan-300/80" />
          </div>
          <ResponsiveContainer width="100%" height={300}>
            {topRisks.length > 0 ? (
              <BarChart data={topRisks.slice(0, 10)}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
                <XAxis 
                  dataKey="email" 
                  angle={-45} 
                  textAnchor="end" 
                  height={100}
                  tick={{ fontSize: 11, fill: '#9ca3af' }}
                />
                <YAxis 
                  tick={{ fontSize: 12, fill: '#9ca3af' }}
                  label={{ value: 'Risk Score', angle: -90, position: 'insideLeft', style: { textAnchor: 'middle', fontSize: '12px' } }}
                />
                <Tooltip 
                  contentStyle={{ 
                    backgroundColor: '#020617', 
                    border: '1px solid rgba(148,163,184,0.5)',
                    borderRadius: '8px',
                    boxShadow: '0 10px 40px rgba(15,23,42,0.9)',
                    color: '#e5e7eb'
                  }}
                  formatter={(value: any, _name: string, props: any) => [
                    `${value} (${props.payload.max_severity})`,
                    'Risk Score'
                  ]}
                />
                <Bar 
                  dataKey="score" 
                  radius={[8, 8, 0, 0]}
                >
                  {topRisks.slice(0, 10).map((entry, index) => (
                    <Cell 
                      key={`cell-${index}`} 
                      fill={
                        entry.max_severity === 'critical' ? '#dc2626' :
                        entry.max_severity === 'high' ? '#ea580c' :
                        entry.max_severity === 'medium' ? '#f59e0b' :
                        '#10b981'
                      } 
                    />
                  ))}
                </Bar>
              </BarChart>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                <p>No risk data available. Run risk computation first.</p>
              </div>
            )}
          </ResponsiveContainer>
        </div>

        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h2 className="text-xl font-semibold text-slate-50">Severity Distribution</h2>
              <p className="text-sm text-slate-400 mt-1">Breakdown by risk level</p>
            </div>
            <ChartBarIcon className="h-6 w-6 text-cyan-300/80" />
          </div>
          <ResponsiveContainer width="100%" height={300}>
            {severityDistribution.some((s) => s.value > 0) ? (
              <PieChart>
                <Pie
                  data={severityDistribution.filter((s) => s.value > 0)}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, value, percent }) => 
                    `${name}: ${value} (${(percent * 100).toFixed(0)}%)`
                  }
                  outerRadius={100}
                  fill="#8884d8"
                  dataKey="value"
                >
                  {severityDistribution.filter((s) => s.value > 0).map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip 
                  contentStyle={{ 
                    backgroundColor: '#fff', 
                    border: '1px solid #e5e7eb',
                    borderRadius: '8px',
                    boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1)'
                  }}
                  formatter={(value: any) => [value, 'Identities']}
                />
                <Legend 
                  verticalAlign="bottom" 
                  height={36}
                  formatter={(value) => {
                    const entry = severityDistribution.find((s) => s.name === value);
                    return entry ? `${value} (${entry.value})` : value;
                  }}
                />
              </PieChart>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                <p>No severity data available. Run risk computation first.</p>
              </div>
            )}
          </ResponsiveContainer>
        </div>
      </div>

      {/* Privileged Identities */}
      {stats && stats.privileged_identities && stats.privileged_identities.length > 0 && showPrivileged && (
        <div id="privileged-section" className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200 bg-blue-50">
            <h2 className="text-xl font-semibold text-gray-900">Privileged Identities</h2>
            <p className="text-sm text-blue-600 mt-1">
              {stats.privileged_identities.length} identities with elevated access (admin, privileged, root, super)
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Identity</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Source Systems</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Risk Score</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {stats.privileged_identities.slice(0, 20).map((identity) => (
                  <tr key={identity.identity_id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center">
                        <div className="flex-shrink-0 h-10 w-10 rounded-full bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center text-white font-semibold text-sm">
                          {(identity.display_name || identity.email)[0].toUpperCase()}
                        </div>
                        <div className="ml-4">
                          <div className="text-sm font-medium text-gray-900">
                            {identity.display_name || identity.email}
                          </div>
                          <div className="text-sm text-gray-500">{identity.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap gap-1">
                        {identity.source_systems.map((system) => (
                          <span
                            key={system}
                            className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                              system.includes('okta') ? 'bg-blue-100 text-blue-800' :
                              system.includes('sailpoint') ? 'bg-purple-100 text-purple-800' :
                              'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {formatSourceSystem(system)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {identity.risk_score !== undefined && identity.max_severity ? (
                        <RiskBadge score={identity.risk_score} severity={identity.max_severity} />
                      ) : (
                        <span className="text-sm text-gray-400">N/A</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <Link
                        to={`/identities/${identity.identity_id}`}
                        className="text-blue-600 hover:text-blue-900 font-medium transition-colors"
                      >
                        View Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Identities Without MFA */}
      {stats && stats.no_mfa_identities && stats.no_mfa_identities.length > 0 && showNoMfa && (
        <div id="no-mfa-section" className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200 bg-yellow-50">
            <h2 className="text-xl font-semibold text-gray-900">Identities Without MFA</h2>
            <p className="text-sm text-gray-600 mt-1">
              {stats.no_mfa_identities.length} privileged identities without MFA enabled
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Identity</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Source Systems</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Risk Score</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {stats.no_mfa_identities.slice(0, 10).map((identity) => (
                  <tr key={identity.identity_id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center">
                        <div className="flex-shrink-0 h-10 w-10 rounded-full bg-gradient-to-br from-yellow-400 to-yellow-600 flex items-center justify-center text-white font-semibold text-sm">
                          {(identity.display_name || identity.email)[0].toUpperCase()}
                        </div>
                        <div className="ml-4">
                          <div className="text-sm font-medium text-gray-900">
                            {identity.display_name || identity.email}
                          </div>
                          <div className="text-sm text-gray-500">{identity.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap gap-1">
                        {identity.source_systems.map((system) => (
                          <span
                            key={system}
                            className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                              system.includes('okta') ? 'bg-blue-100 text-blue-800' :
                              system.includes('sailpoint') ? 'bg-purple-100 text-purple-800' :
                              'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {formatSourceSystem(system)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {identity.risk_score !== undefined && identity.max_severity ? (
                        <RiskBadge score={identity.risk_score} severity={identity.max_severity} />
                      ) : (
                        <span className="text-sm text-gray-400">N/A</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <Link
                        to={`/identities/${identity.identity_id}`}
                        className="text-blue-600 hover:text-blue-900 font-medium transition-colors"
                      >
                        View Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Deadend Identities */}
      {stats && stats.deadend_identities && stats.deadend_identities.length > 0 && showDeadends && (
        <div id="deadend-section" className="bg-white border border-red-200 rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200 bg-red-50">
            <h2 className="text-xl font-semibold text-gray-900">Deadend Identities</h2>
            <p className="text-sm text-red-600 mt-1">
              {stats.deadend_identities.length} identities with critical deadend issues
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Identity</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Why deadend</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Source Systems</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Risk Score</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {stats.deadend_identities.slice(0, 10).map((identity) => (
                  <tr key={identity.identity_id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center">
                        <ExclamationTriangleIcon className="h-5 w-5 text-red-500 mr-2" />
                        <div className="flex-shrink-0 h-10 w-10 rounded-full bg-gradient-to-br from-red-400 to-red-600 flex items-center justify-center text-white font-semibold text-sm">
                          {(identity.display_name || identity.email)[0].toUpperCase()}
                        </div>
                        <div className="ml-4">
                          <div className="text-sm font-medium text-gray-900">
                            {identity.display_name || identity.email}
                          </div>
                          <div className="text-sm text-gray-500">{identity.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4 max-w-md">
                      <p className="text-sm text-gray-700">{identity.deadend_reason || '—'}</p>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap gap-1">
                        {identity.source_systems.map((system) => (
                          <span
                            key={system}
                            className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                              system.includes('okta') ? 'bg-blue-100 text-blue-800' :
                              system.includes('sailpoint') ? 'bg-purple-100 text-purple-800' :
                              'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {formatSourceSystem(system)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {identity.risk_score !== undefined && identity.max_severity ? (
                        <RiskBadge score={identity.risk_score} severity={identity.max_severity} />
                      ) : (
                        <span className="text-sm text-gray-400">N/A</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <Link
                        to={`/identities/${identity.identity_id}`}
                        className="text-blue-600 hover:text-blue-900 font-medium transition-colors"
                      >
                        View Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Cross-Cloud Admins */}
      {stats && stats.cross_cloud_admin_list && stats.cross_cloud_admin_list.length > 0 && showCrossCloud && (
        <div id="cross-cloud-section" className="bg-white border border-orange-200 rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200 bg-orange-50">
            <h2 className="text-xl font-semibold text-gray-900">Cross-Cloud Admins</h2>
            <p className="text-sm text-orange-600 mt-1">
              {stats.cross_cloud_admin_list.length} identities with admin access across multiple systems
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Identity</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Source Systems</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Risk Score</th>
                  <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {stats.cross_cloud_admin_list.slice(0, 10).map((identity) => (
                  <tr key={identity.identity_id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center">
                        <div className="flex-shrink-0 h-10 w-10 rounded-full bg-gradient-to-br from-orange-400 to-orange-600 flex items-center justify-center text-white font-semibold text-sm">
                          {(identity.display_name || identity.email)[0].toUpperCase()}
                        </div>
                        <div className="ml-4">
                          <div className="text-sm font-medium text-gray-900">
                            {identity.display_name || identity.email}
                          </div>
                          <div className="text-sm text-gray-500">{identity.email}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap gap-1">
                        {identity.source_systems.map((system) => (
                          <span
                            key={system}
                            className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                              system.includes('okta') ? 'bg-blue-100 text-blue-800' :
                              system.includes('sailpoint') ? 'bg-purple-100 text-purple-800' :
                              'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {formatSourceSystem(system)}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {identity.risk_score !== undefined && identity.max_severity ? (
                        <RiskBadge score={identity.risk_score} severity={identity.max_severity} />
                      ) : (
                        <span className="text-sm text-gray-400">N/A</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <Link
                        to={`/identities/${identity.identity_id}`}
                        className="text-blue-600 hover:text-blue-900 font-medium transition-colors"
                      >
                        View Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Top Risky Identities */}
      <div className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-200 bg-gray-50">
          <h2 className="text-xl font-semibold text-gray-900">Top Risky Identities</h2>
          <p className="text-sm text-gray-500 mt-1">Identities requiring immediate attention</p>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Identity</th>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Source Systems</th>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Risk Score</th>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Severity</th>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Flags</th>
                <th className="px-6 py-3 text-left text-xs font-semibold text-gray-700 uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {topRisks.map((identity) => (
                <tr key={identity.identity_id} className="hover:bg-gray-50 transition-colors">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="flex items-center">
                      <div className="flex-shrink-0 h-10 w-10 rounded-full bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center text-white font-semibold text-sm">
                        {(identity.display_name || identity.email)[0].toUpperCase()}
                      </div>
                      <div className="ml-4">
                        <div className="text-sm font-medium text-gray-900">
                          {identity.display_name || identity.email}
                        </div>
                        <div className="text-sm text-gray-500">{identity.email}</div>
                      </div>
                    </div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="flex flex-wrap gap-1">
                      {identity.source_systems && identity.source_systems.length > 0 ? (
                        identity.source_systems.map((system) => (
                          <span
                            key={system}
                            className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
                              system.includes('okta') ? 'bg-blue-100 text-blue-800' :
                              system.includes('sailpoint') ? 'bg-purple-100 text-purple-800' :
                              system.includes('azure') || system.includes('entra') ? 'bg-blue-200 text-blue-900' :
                              'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {formatSourceSystem(system)}
                          </span>
                        ))
                      ) : (
                        <span className="text-xs text-gray-400">No systems</span>
                      )}
                    </div>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <RiskBadge score={identity.score} severity={identity.max_severity} />
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium capitalize ${
                      identity.max_severity === 'critical' ? 'bg-red-100 text-red-800' :
                      identity.max_severity === 'high' ? 'bg-orange-100 text-orange-800' :
                      identity.max_severity === 'medium' ? 'bg-yellow-100 text-yellow-800' :
                      'bg-green-100 text-green-800'
                    }`}>
                      {identity.max_severity}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="flex items-center">
                      <ExclamationTriangleIcon className="h-4 w-4 text-orange-500 mr-1" />
                      <span className="text-sm font-medium text-gray-900">{identity.flag_count}</span>
                    </div>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                    <Link
                      to={`/identities/${identity.identity_id}`}
                      className="text-blue-600 hover:text-blue-900 font-medium transition-colors"
                    >
                      View Details →
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
