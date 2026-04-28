package sailpoint

import "fmt"

// AuthConfig defines SailPoint authentication strategy
type AuthConfig struct {
	AuthType       string   // "oauth2_client_credentials"
	RequiredScopes []string // Required OAuth scopes
	RequiredRoles  []string // Required SailPoint roles (if applicable)
}

// SailPointAuthConfig is the authentication configuration for SailPoint connector
var SailPointAuthConfig = AuthConfig{
	AuthType: "oauth2_client_credentials",
	RequiredScopes: []string{
		"sp:scopes:read",
		"sp:identities:read",
		"sp:access-profiles:read",
		"sp:entitlements:read",
		"sp:roles:read",
		"sp:certifications:read",
	},
	RequiredRoles: []string{
		"Administrator",        // Full access
		"Identity Administrator", // Identity management
		"Security Administrator", // Security operations
		"Access Administrator",   // Access management
	},
}

// ValidateAuth validates authentication configuration
func ValidateAuth(tenant, clientID, clientSecret string) error {
	if tenant == "" {
		return fmt.Errorf("SAILPOINT_TENANT is required")
	}
	if clientID == "" {
		return fmt.Errorf("SAILPOINT_CLIENT_ID is required")
	}
	if clientSecret == "" {
		return fmt.Errorf("SAILPOINT_CLIENT_SECRET is required")
	}
	return nil
}
