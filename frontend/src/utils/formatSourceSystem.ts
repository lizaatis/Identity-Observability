/**
 * Maps internal source_system keys to human-readable, relevant display names.
 * Uses names that reflect what the system is (e.g. Okta Admin, AWS IAM).
 */
const SOURCE_SYSTEM_LABELS: Record<string, string> = {
  // Okta – production as "Okta Admin" (main IdP), mock as test
  okta_mock: 'Okta (Test)',
  okta_prod: 'Okta Admin',
  okta_primary: 'Okta Admin',
  // Microsoft Entra (Azure AD)
  entra_mock: 'Microsoft Entra ID (Test)',
  entra_prod: 'Microsoft Entra ID',
  azure_ad: 'Microsoft Entra ID',
  // AWS
  aws_mock: 'AWS IAM (Test)',
  aws_prod: 'AWS IAM',
  // SailPoint
  sailpoint_mock: 'SailPoint Identity (Test)',
  sailpoint_prod: 'SailPoint Identity',
  // Google Cloud / Workspace
  gcp_mock: 'Google Cloud IAM (Test)',
  gcp_prod: 'Google Cloud IAM',
  google_workspace: 'Google Workspace',
};

/**
 * Returns a human-readable label for a source system key.
 * Falls back to a formatted version of the key (e.g. "some_system" -> "Some System").
 */
export function formatSourceSystem(system: string): string {
  if (!system) return '—';
  const label = SOURCE_SYSTEM_LABELS[system.toLowerCase()];
  if (label) return label;
  // Fallback: capitalize words and replace underscores with spaces
  return system
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}
