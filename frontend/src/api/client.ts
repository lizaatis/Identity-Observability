import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Types
export interface Identity {
  id: number;
  employee_id?: string;
  email: string;
  display_name?: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface IdentitySource {
  source_system: string;
  source_user_id: string;
  source_status: string;
  synced_at: string;
  mfa_enabled?: string;
  data_freshness?: string;
}

export interface SystemSummaryItem {
  system: string;
  is_admin: boolean;
  admin_roles: string[];
  mfa_enabled?: boolean;
  last_sync_at: string;
  data_freshness?: string;
}

export interface StitchingSummary {
  confidence: string;
  reasons: string[];
  needs_review: boolean;
  source_count: number;
}

export interface DeadendSummary {
  count: number;
  reasons: string[];
}

export interface EffectivePermission {
  permission_id: number;
  permission_name: string;
  resource_type?: string;
  permission_source_system: string;
  path_type: string;
  role_id: number;
  role_name: string;
  privilege_level: string;
  role_source_system: string;
  group_id?: number;
  group_name?: string;
}

export interface Hop {
  hop_type: string;
  hop_name: string;
  hop_detail?: string;
  ord: number;
}

export interface PermissionLineage {
  permission_id: number;
  permission_name: string;
  path: Hop[];
}

export interface IdentityDetail {
  identity: Identity;
  sources: IdentitySource[];
  effective_permissions: EffectivePermission[];
  lineage: PermissionLineage[];
  system_summary?: SystemSummaryItem[];
  deadend_summary?: DeadendSummary;
  stitching_summary?: StitchingSummary;
}

export interface RiskFlag {
  rule_key: string;
  severity: string;
  is_deadend: boolean;
  message: string;
  metadata?: Record<string, any>;
}

export interface RiskScore {
  identity_id: number;
  score: number;
  max_severity: string;
  computed_at: string;
  flags: RiskFlag[];
}

export interface TopRiskIdentity {
  identity_id: number;
  email: string;
  display_name?: string;
  score: number;
  max_severity: string;
  flag_count: number;
  source_systems: string[];
}

export interface ConnectorStatus {
  connector_id: string;
  source_system: string;
  connector_name: string;
  status: string;
  last_sync?: {
    id: number;
    started_at: string;
    finished_at?: string;
    status: string;
    error_count: number;
    warning_count: number;
    last_error?: string;
    duration?: string;
  };
  recent_syncs: Array<{
    id: number;
    started_at: string;
    finished_at?: string;
    status: string;
    error_count: number;
    warning_count: number;
    last_error?: string;
    duration?: string;
  }>;
}

export interface ConnectorFullStatusItem {
  connector_id: string;
  source_system: string;
  connector_name: string;
  last_sync_at?: string;
  status: string;
  last_sync?: ConnectorStatus['last_sync'];
  recent_syncs?: ConnectorStatus['recent_syncs'];
  sync_metadata?: Record<string, unknown>;
  row_counts?: Record<string, unknown>;
}

// API functions
export const identityAPI = {
  list: async (params?: { page?: number; page_size?: number; source_system?: string; status?: string }) => {
    const response = await apiClient.get('/identities', { params });
    return response.data;
  },
  get: async (id: number) => {
    const response = await apiClient.get(`/identities/${id}`);
    return response.data as IdentityDetail;
  },
  getEffectivePermissions: async (id: number) => {
    const response = await apiClient.get(`/identities/${id}/effective-permissions`);
    return response.data;
  },
  getRisk: async (id: number, recompute = false) => {
    const response = await apiClient.get(`/identities/${id}/risk`, { params: { recompute } });
    return response.data as RiskScore;
  },
};

export const riskAPI = {
  getTop: async (limit = 10) => {
    const response = await apiClient.get('/risk/top', { params: { limit } });
    return response.data;
  },
};

export interface IdentitySummary {
  identity_id: number;
  email: string;
  display_name?: string;
  source_systems: string[];
  risk_score?: number;
  max_severity?: string;
  deadend_reason?: string;
}

export interface DashboardStats {
  total_identities: number;
  privileged_count: number;
  cross_cloud_admins: number;
  mfa_coverage: number;
  deadend_count: number;
  identities_by_system: Record<string, number>;
  risk_distribution: Record<string, number>;
  recent_syncs: Array<{
    source_system: string;
    connector_name: string;
    last_sync_at: string;
    status: string;
    identity_count: number;
  }>;
  top_risk_issues: Array<{
    rule_key: string;
    severity: string;
    count: number;
    is_deadend: boolean;
    message: string;
  }>;
  no_mfa_identities: IdentitySummary[];
  deadend_identities: IdentitySummary[];
  cross_cloud_admin_list: IdentitySummary[];
  privileged_identities: IdentitySummary[];
}

export const dashboardAPI = {
  getStats: async () => {
    const response = await apiClient.get('/dashboard/stats');
    return response.data as DashboardStats;
  },
};

export interface PlatformDiagnostics {
  database_ok: boolean;
  identity_events_table: boolean;
  identity_sources_metadata_column: boolean;
  neo4j_configured: boolean;
  neo4j_reachable: boolean;
  identity_count: number;
  messages: string[];
}

export const platformAPI = {
  getDiagnostics: async () => {
    const response = await apiClient.get('/platform/diagnostics');
    return response.data as PlatformDiagnostics;
  },
};

export const connectorAPI = {
  list: async () => {
    const response = await apiClient.get('/connectors');
    return response.data.connectors as Array<{
      connector_id: string;
      source_system: string;
      connector_name: string;
      last_sync_at?: string;
      status: string;
    }>;
  },
  listFullStatus: async () => {
    const response = await apiClient.get('/connectors/full-status');
    return response.data.connectors as ConnectorFullStatusItem[];
  },
  getStatus: async (id: string) => {
    const response = await apiClient.get(`/connectors/${id}/status`);
    return response.data as ConnectorStatus;
  },
  testOkta: async (domain: string, token: string) => {
    const response = await apiClient.post('/connectors/test/okta', { domain, token });
    return response.data as { ok: boolean; error?: string; message?: string };
  },
  testSailPoint: async (tenant: string, client_id: string, secret: string) => {
    const response = await apiClient.post('/connectors/test/sailpoint', { tenant, client_id, secret });
    return response.data as { ok: boolean; error?: string; message?: string };
  },
  testGCP: async (project_id: string) => {
    const response = await apiClient.post('/connectors/test/gcp', { project_id });
    return response.data as { ok: boolean; error?: string; message?: string };
  },
};

export interface ChangeItem {
  id: number;
  event_time: string;
  source_system: string;
  event_type: string;
  identity_id?: number;
  email?: string;
  display_message?: string;
  summary: string;
}

export const changesAPI = {
  get: async (params?: { since?: string; identity?: string }) => {
    const response = await apiClient.get('/changes', { params });
    return response.data as { changes: ChangeItem[] };
  },
};

export interface LensResponse {
  lens: string;
  count: number;
  items: IdentitySummary[];
}

export const lensesAPI = {
  privileged: async (params?: { source_system?: string; limit?: number }) => {
    const response = await apiClient.get('/lenses/privileged', { params });
    return response.data as LensResponse;
  },
  crossCloudAdmins: async (params?: { limit?: number }) => {
    const response = await apiClient.get('/lenses/cross-cloud-admins', { params });
    return response.data as LensResponse;
  },
  deadends: async (params?: { limit?: number }) => {
    const response = await apiClient.get('/lenses/deadends', { params });
    return response.data as LensResponse;
  },
  noMfa: async (params?: { source_system?: string; limit?: number }) => {
    const response = await apiClient.get('/lenses/no-mfa', { params });
    return response.data as LensResponse;
  },
};

export const exportAPI = {
  exportIdentity: async (id: number, format: 'csv' | 'pdf') => {
    const response = await apiClient.get(`/export/identities/${id}`, {
      params: { format },
      responseType: 'blob',
    });
    return response.data;
  },
  exportHighRisk: async (format: 'csv' | 'pdf') => {
    const response = await apiClient.get('/export/risk/high-risk', {
      params: { format },
      responseType: 'blob',
    });
    return response.data;
  },
  exportDeadends: async (format: 'csv' | 'pdf') => {
    const response = await apiClient.get('/export/risk/deadends', {
      params: { format },
      responseType: 'blob',
    });
    return response.data;
  },
};

export interface TimelineEvent {
  event_id: number;
  event_time: string;
  source_system: string;
  event_type: string;
  event_category: string;
  display_message?: string;
  event_data: Record<string, any>;
}

export interface TimelineResponse {
  identity_id: number;
  events: TimelineEvent[];
  total: number;
  page: number;
  page_size: number;
}

export interface WeeklyRiskVelocity {
  week_start: string;
  high_risk_event_count: number;
  event_types: string[];
  last_event_time: string;
}

export interface RiskVelocityResponse {
  identity_id: number;
  current_week_count: number;
  previous_week_count: number;
  velocity_trend: 'increasing' | 'decreasing' | 'stable';
  weekly_breakdown: WeeklyRiskVelocity[];
}

export const timelineAPI = {
  getTimeline: async (id: number, page = 1, pageSize = 100) => {
    const response = await apiClient.get(`/identities/${id}/timeline`, {
      params: { page, page_size: pageSize },
    });
    return response.data as TimelineResponse;
  },
  getRiskVelocity: async (id: number) => {
    const response = await apiClient.get(`/identities/${id}/risk-velocity`);
    return response.data as RiskVelocityResponse;
  },
};

export interface BlastRadiusNode {
  id: string;
  label: string;
  type: string;
  properties: Record<string, any>;
}

export interface BlastRadiusEdge {
  id: string;
  source: string;
  target: string;
  type: string;
  label: string;
  distance: number;
}

export interface BlastRadiusResponse {
  identity_id: number;
  max_hops: number;
  nodes: BlastRadiusNode[];
  edges: BlastRadiusEdge[];
  stats: {
    total_nodes: number;
    total_edges: number;
    groups_reachable: number;
    roles_reachable: number;
    permissions_reachable: number;
    resources_reachable: number;
  };
  message?: string; // e.g. when Neo4j is not connected
}

export interface ToxicComboMatch {
  rule_name: string;
  severity: string;
  matches: Array<Record<string, any>>;
  count: number;
}

export interface IQLSearchResult {
  query: string;
  results: Array<Record<string, any>>;
  count: number;
}

export interface GraphStatusResponse {
  neo4j_reachable: boolean;
  last_sync_at?: string;
  duration_ms?: number;
  node_count?: number;
  edge_count?: number;
  last_error?: string;
}

export const graphAPI = {
  getBlastRadius: async (id: number, hops = 3) => {
    const response = await apiClient.get(`/graph/blast-radius/${id}`, {
      params: { hops },
    });
    return response.data as BlastRadiusResponse;
  },
  getGraphStatus: async () => {
    const response = await apiClient.get('/graph/status');
    return response.data as GraphStatusResponse;
  },
  syncGraph: async () => {
    const response = await apiClient.post('/graph/sync');
    return response.data;
  },
  getToxicCombo: async () => {
    const response = await apiClient.get('/graph/toxic-combo');
    return response.data.matches as ToxicComboMatch[];
  },
  iqlSearch: async (query: string) => {
    const response = await apiClient.get('/iql/search', {
      params: { q: query },
    });
    return response.data as IQLSearchResult;
  },
  getIQLFields: async () => {
    const response = await apiClient.get('/iql/fields');
    return response.data;
  },
};

export interface RemediationAction {
  id: number;
  identity_id: number;
  action_type: string;
  target_system: string;
  status: string;
  requested_by: string;
  requested_at: string;
  approved_by?: string;
  approved_at?: string;
  executed_at?: string;
  error_message?: string;
  metadata?: Record<string, any>;
}

export const remediationAPI = {
  createAction: async (identityId: number, action: {
    action_type: string;
    target_system: string;
    requires_approval: boolean;
    metadata?: Record<string, any>;
  }) => {
    const response = await apiClient.post(`/identities/${identityId}/remediate`, action);
    return response.data as RemediationAction;
  },
  approve: async (actionId: number) => {
    const response = await apiClient.post(`/remediation/${actionId}/approve`);
    return response.data;
  },
  reject: async (actionId: number) => {
    const response = await apiClient.post(`/remediation/${actionId}/reject`);
    return response.data;
  },
};

export interface CustomRule {
  id: number;
  name: string;
  description: string;
  rule_yaml: string;
  severity: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
  last_tested_at?: string;
  test_results?: Record<string, any>;
}

export const customRulesAPI = {
  list: async () => {
    const response = await apiClient.get('/custom-rules');
    return response.data as { rules: CustomRule[] };
  },
  create: async (rule: {
    name: string;
    description?: string;
    rule_yaml: string;
    severity: string;
  }) => {
    const response = await apiClient.post('/custom-rules', rule);
    return response.data;
  },
  test: async (ruleId: number) => {
    const response = await apiClient.post(`/custom-rules/${ruleId}/test`);
    return response.data;
  },
};

export interface ExecutiveDashboardData {
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

export const executiveDashboardAPI = {
  getExecutive: async () => {
    const response = await apiClient.get('/dashboard/executive');
    return response.data as ExecutiveDashboardData;
  },
};
