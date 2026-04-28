package okta

import "fmt"

// AuthConfig defines Okta authentication strategy
type AuthConfig struct {
	AuthType       string   // "api_token"
	RequiredScopes []string // Required OAuth scopes (for future OAuth support)
	RequiredRoles  []string // Required Okta admin roles
}

// OktaAuthConfig is the authentication configuration for Okta connector
var OktaAuthConfig = AuthConfig{
	AuthType: "api_token",
	RequiredScopes: []string{
		"okta.users.read",
		"okta.groups.read",
		"okta.roles.read",
		"okta.apps.read",
	},
	RequiredRoles: []string{
		"SUPER_ADMIN",        // Full access
		"ORG_ADMIN",          // Organization admin
		"API_ACCESS_MANAGEMENT_ADMIN", // API access management
		"READ_ONLY_ADMIN",    // Read-only access (minimum)
	},
}

// ValidateAuth validates authentication configuration
func ValidateAuth(domain, apiToken string) error {
	if domain == "" {
		return fmt.Errorf("OKTA_DOMAIN is required")
	}
	if apiToken == "" {
		return fmt.Errorf("OKTA_API_TOKEN is required")
	}
	return nil
}
