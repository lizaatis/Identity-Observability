import { useEffect, useState } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, BarChart, Bar } from 'recharts';
import { executiveDashboardAPI } from '../api/client';

interface ExecutiveDashboardData {
  risk_trend: Array<{
    date: string;
    total: number;
    critical: number;
    high: number;
    medium: number;
    low: number;
  }>;
  top_remediated: Array<{
    risk_type: string;
    count: number;
    avg_resolution_time: string;
  }>;
  compliance_posture: {
    critical_resolved_24h: number;
    all_risks_resolved_7d: number;
    compliance_score: number;
  };
  total_risks: number;
  critical_risks: number;
  resolved_24h: number;
  resolved_7d: number;
}

export default function ExecutiveDashboard() {
  const [data, setData] = useState<ExecutiveDashboardData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      try {
        const result = await executiveDashboardAPI.getExecutive();
        setData(result);
      } catch (error) {
        console.error('Failed to load executive dashboard:', error);
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[300px]">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-10 w-10 border-b-2 border-cyan-400" />
          <p className="mt-4 text-slate-300 text-sm">Loading executive dashboard…</p>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="text-center py-12 text-rose-400">
        Failed to load executive dashboard.
      </div>
    );
  }

  // Defensive defaults so the page always shows something
  const riskTrend = Array.isArray(data.risk_trend) ? data.risk_trend : [];
  const topRemediated = Array.isArray(data.top_remediated) ? data.top_remediated : [];
  const compliance = data.compliance_posture ?? {
    critical_resolved_24h: 0,
    all_risks_resolved_7d: 0,
    compliance_score: 0,
  };
  const totalRisks = typeof data.total_risks === 'number' ? data.total_risks : 0;
  const criticalRisks = typeof data.critical_risks === 'number' ? data.critical_risks : 0;

  return (
    <div className="space-y-6">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <h1 className="text-2xl font-semibold mb-1 text-slate-50">Executive Summary</h1>
        <p className="text-sm text-slate-400">Identity risk and compliance overview.</p>
      </div>

      {/* Compliance Posture */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-5">
          <div className="text-xs text-slate-400">Compliance Score</div>
          <div
            className="text-3xl font-bold"
            style={{ color: getScoreColor(compliance.compliance_score) }}
          >
            {compliance.compliance_score.toFixed(1)}%
          </div>
        </div>
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-5">
          <div className="text-xs text-slate-400">Critical Resolved (24h)</div>
          <div className="text-3xl font-bold text-emerald-400">
            {compliance.critical_resolved_24h.toFixed(1)}%
          </div>
        </div>
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-5">
          <div className="text-xs text-slate-400">All Risks Resolved (7d)</div>
          <div className="text-3xl font-bold text-sky-400">
            {compliance.all_risks_resolved_7d.toFixed(1)}%
          </div>
        </div>
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-5">
          <div className="text-xs text-slate-400">Total Risks</div>
          <div className="text-3xl font-bold text-slate-50">{totalRisks}</div>
          <div className="text-xs text-rose-400 mt-1">{criticalRisks} critical</div>
        </div>
      </div>

      {/* Risk Trend */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
        <h2 className="text-lg font-semibold mb-2 text-slate-50">Risk Trend (Last 30 Days)</h2>
        <p className="text-sm text-slate-400 mb-4">
          Count of identities by highest risk severity per day (from risk scores).
        </p>
        {riskTrend.length > 0 ? (
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={riskTrend} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
              <XAxis
                dataKey="date"
                tickFormatter={(v) => {
                  try {
                    const d = new Date(v);
                    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
                  } catch {
                    return v;
                  }
                }}
              />
              <YAxis
                domain={[0, 'auto']}
                allowDataOverflow
                tick={{ fontSize: 12, fill: '#9ca3af' }}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: '#020617',
                  border: '1px solid rgba(148,163,184,0.5)',
                  borderRadius: 8,
                  color: '#e5e7eb',
                }}
                labelFormatter={(v) =>
                  new Date(v).toLocaleDateString(undefined, { dateStyle: 'medium' })
                }
              />
              <Legend />
              <Line type="monotone" dataKey="total" stroke="#6366f1" name="Total" strokeWidth={2} />
              <Line type="monotone" dataKey="critical" stroke="#dc2626" name="Critical" />
              <Line type="monotone" dataKey="high" stroke="#ea580c" name="High" />
              <Line type="monotone" dataKey="medium" stroke="#f59e0b" name="Medium" />
              <Line type="monotone" dataKey="low" stroke="#10b981" name="Low" />
            </LineChart>
          </ResponsiveContainer>
        ) : (
          <div className="py-10 text-center rounded-2xl bg-slate-950/80 border border-slate-800/80">
            <p className="text-slate-300 font-medium text-sm">No risk trend data yet.</p>
            <p className="text-xs text-slate-500 mt-1">
              Run the risk engine to see trends over time.
            </p>
          </div>
        )}
      </div>

      {/* Top Remediated Issues */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
        <h2 className="text-lg font-semibold mb-4 text-slate-50">Top Remediated Issues</h2>
        {topRemediated.length > 0 ? (
          <>
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={topRemediated}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
                <XAxis
                  dataKey="risk_type"
                  tick={{ fontSize: 12, fill: '#9ca3af' }}
                />
                <YAxis tick={{ fontSize: 12, fill: '#9ca3af' }} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#020617',
                    border: '1px solid rgba(148,163,184,0.5)',
                    borderRadius: 8,
                    color: '#e5e7eb',
                  }}
                />
                <Bar dataKey="count" fill="#22d3ee" />
              </BarChart>
            </ResponsiveContainer>
            <div className="mt-4 space-y-2">
              {topRemediated.map((issue, idx) => (
                <div
                  key={idx}
                  className="flex justify-between items-center p-2 bg-slate-900/80 rounded border border-slate-800/80"
                >
                  <div>
                    <div className="font-medium text-slate-100">{issue.risk_type}</div>
                    <div className="text-xs text-slate-400">
                      Avg resolution: {issue.avg_resolution_time}
                    </div>
                  </div>
                  <div className="text-2xl font-bold text-slate-50">{issue.count}</div>
                </div>
              ))}
            </div>
          </>
        ) : (
          <div className="py-10 text-center rounded-2xl bg-slate-950/80 border border-slate-800/80">
            <p className="text-slate-300 font-medium text-sm">No remediated issues yet.</p>
            <p className="text-xs text-slate-500 mt-1">
              Resolved actions from the Action Hub (Jira tickets, Slack approvals, or direct remediations) will appear here.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function getScoreColor(score: number): string {
  if (score >= 80) return '#10b981'; // green
  if (score >= 60) return '#f59e0b'; // yellow
  return '#ef4444'; // red
}
