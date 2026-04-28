import { useEffect, useState } from 'react';
import { timelineAPI, TimelineEvent, RiskVelocityResponse } from '../api/client';
import { ClockIcon, ExclamationTriangleIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import { formatSourceSystem } from '../utils/formatSourceSystem';

interface TimelineProps {
  identityId: number;
}

export default function Timeline({ identityId }: TimelineProps) {
  const [events, setEvents] = useState<TimelineEvent[]>([]);
  const [riskVelocity, setRiskVelocity] = useState<RiskVelocityResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const pageSize = 50;

  useEffect(() => {
    const loadData = async () => {
      try {
        setLoading(true);
        const [timelineData, velocityData] = await Promise.all([
          timelineAPI.getTimeline(identityId, page, pageSize),
          timelineAPI.getRiskVelocity(identityId),
        ]);
        setEvents(timelineData.events);
        setTotal(timelineData.total);
        setRiskVelocity(velocityData);
      } catch (error) {
        console.error('Failed to load timeline:', error);
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, [identityId, page]);

  const getEventIcon = (category: string) => {
    switch (category) {
      case 'lifecycle':
        return <ClockIcon className="h-5 w-5 text-blue-500" />;
      case 'mfa':
        return <CheckCircleIcon className="h-5 w-5 text-green-500" />;
      case 'group_membership':
        return <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500" />;
      case 'account':
        return <XCircleIcon className="h-5 w-5 text-red-500" />;
      default:
        return <ClockIcon className="h-5 w-5 text-gray-500" />;
    }
  };

  const getEventColor = (category: string) => {
    switch (category) {
      case 'lifecycle':
        return 'bg-blue-50 border-blue-200';
      case 'mfa':
        return 'bg-green-50 border-green-200';
      case 'group_membership':
        return 'bg-yellow-50 border-yellow-200';
      case 'account':
        return 'bg-red-50 border-red-200';
      default:
        return 'bg-gray-50 border-gray-200';
    }
  };

  const formatEventType = (eventType: string) => {
    return eventType
      .split('.')
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ');
  };

  if (loading && events.length === 0) {
    return (
      <div className="flex items-center justify-center min-h-[200px]">
        <div className="text-center">
          <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-cyan-400" />
          <p className="mt-3 text-sm text-slate-300">Loading timeline…</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Risk Velocity Card */}
      {riskVelocity && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
          <h2 className="text-lg font-semibold text-slate-50 mb-3">Risk Velocity</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
            <div className="border rounded-lg p-4">
              <div className="text-xs text-slate-400">Current Week</div>
              <div className="text-2xl font-bold text-slate-50">{riskVelocity.current_week_count}</div>
              <div className="text-[11px] text-slate-500">High-risk events</div>
            </div>
            <div className="border rounded-lg p-4">
              <div className="text-xs text-slate-400">Previous Week</div>
              <div className="text-2xl font-bold text-slate-50">{riskVelocity.previous_week_count}</div>
              <div className="text-[11px] text-slate-500">High-risk events</div>
            </div>
            <div className="border rounded-lg p-4">
              <div className="text-xs text-slate-400">Trend</div>
              <div className="text-2xl font-bold capitalize text-slate-50">
                {riskVelocity.velocity_trend}
              </div>
              <div
                className={`text-xs ${
                  riskVelocity.velocity_trend === 'increasing'
                    ? 'text-rose-400'
                    : riskVelocity.velocity_trend === 'decreasing'
                    ? 'text-emerald-400'
                    : 'text-slate-400'
                }`}
              >
                {riskVelocity.velocity_trend === 'increasing' && '⚠️ Risk increasing'}
                {riskVelocity.velocity_trend === 'decreasing' && '✓ Risk decreasing'}
                {riskVelocity.velocity_trend === 'stable' && '→ Stable'}
              </div>
            </div>
          </div>
          {riskVelocity.weekly_breakdown.length > 0 && (
            <div>
              <h3 className="text-sm font-medium text-slate-100 mb-2">
                Weekly Breakdown (Last 8 Weeks)
              </h3>
              <div className="space-y-2">
                {riskVelocity.weekly_breakdown.map((week, idx) => (
                  <div
                    key={idx}
                    className="flex items-center justify-between p-2 bg-slate-900/80 rounded border border-slate-800/80"
                  >
                    <div>
                      <div className="text-sm font-medium text-slate-100">
                        {new Date(week.week_start).toLocaleDateString()}
                      </div>
                      <div className="text-xs text-slate-400">
                        {week.event_types.slice(0, 3).join(', ')}
                        {week.event_types.length > 3 && '...'}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-lg font-bold text-slate-50">
                        {week.high_risk_event_count}
                      </div>
                      <div className="text-xs text-slate-400">events</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Timeline Events */}
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-semibold text-slate-50">Event Timeline</h2>
          <div className="text-xs text-slate-400">
            {total} total events
          </div>
        </div>

        {events.length === 0 ? (
          <div className="text-center py-12 text-slate-400 text-sm">
            No events found for this identity.
          </div>
        ) : (
          <div className="space-y-4">
            {events.map((event) => (
              <div
                key={event.event_id}
                className={`border-l-4 rounded-lg p-4 ${getEventColor(event.event_category)}`}
              >
                <div className="flex items-start space-x-3">
                  <div className="flex-shrink-0 mt-1">{getEventIcon(event.event_category)}</div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="text-sm font-medium text-slate-100">
                          {event.display_message || formatEventType(event.event_type)}
                        </p>
                        <p className="text-xs text-slate-400 mt-1">
                          {event.event_type} • {formatSourceSystem(event.source_system)}
                        </p>
                      </div>
                      <div className="text-right">
                        <p className="text-xs text-slate-400">
                          {new Date(event.event_time).toLocaleString()}
                        </p>
                        <span
                          className={`inline-block mt-1 px-2 py-0.5 text-xs rounded ${
                            event.event_category === 'account'
                              ? 'bg-rose-500/20 text-rose-200 border border-rose-400/60'
                              : event.event_category === 'mfa'
                              ? 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/60'
                              : 'bg-slate-700/80 text-slate-200 border border-slate-500/60'
                          }`}
                        >
                          {event.event_category}
                        </span>
                      </div>
                    </div>
                    {event.event_data && Object.keys(event.event_data).length > 0 && (
                      <details className="mt-2">
                        <summary className="text-xs text-slate-400 cursor-pointer hover:text-slate-200">
                          View event details
                        </summary>
                        <pre className="mt-2 text-xs bg-slate-900/80 p-2 rounded overflow-auto max-h-40 text-slate-100 border border-slate-800/80">
                          {JSON.stringify(event.event_data, null, 2)}
                        </pre>
                      </details>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination */}
        {total > pageSize && (
          <div className="mt-6 flex items-center justify-between">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              className="px-4 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Previous
            </button>
            <span className="text-sm text-gray-500">
              Page {page} of {Math.ceil(total / pageSize)}
            </span>
            <button
              onClick={() => setPage((p) => p + 1)}
              disabled={page >= Math.ceil(total / pageSize)}
              className="px-4 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Next
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
