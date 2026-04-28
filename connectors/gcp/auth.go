package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/cloudresourcemanager/v1"
)

// AuthConfig defines GCP authentication strategy
type AuthConfig struct {
	AuthType       string   // "service_account"
	RequiredRoles  []string // Required IAM roles
	RequiredScopes []string // Required OAuth scopes
}

// GCPAuthConfig is the authentication configuration for GCP connector
var GCPAuthConfig = AuthConfig{
	AuthType: "service_account",
	RequiredRoles: []string{
		"roles/viewer",
		"roles/iam.securityReviewer",
		"roles/cloudidentity.groups.reader",
		"roles/resourcemanager.organizationViewer",
	},
	RequiredScopes: []string{
		"https://www.googleapis.com/auth/cloud-platform.read-only",
		"https://www.googleapis.com/auth/cloudidentity.groups.readonly",
		"https://www.googleapis.com/auth/cloudidentity.users.readonly",
		"https://www.googleapis.com/auth/iam.readonly",
		"https://www.googleapis.com/auth/cloudresourcemanager.readonly",
	},
}

// GCPClient wraps GCP API clients
type GCPClient struct {
	CloudIdentityService  *cloudidentity.Service
	IAMService            *iam.Service
	ResourceManagerService *cloudresourcemanager.Service
	ProjectID             string
}

// NewGCPClient creates authenticated GCP API clients
func NewGCPClient(ctx context.Context, serviceAccountPath, serviceAccountJSON, projectID string) (*GCPClient, error) {
	var credentials *google.Credentials
	var err error

	// Load credentials from file or JSON string
	if serviceAccountPath != "" {
		// Read file and use CredentialsFromJSON
		data, err := os.ReadFile(serviceAccountPath)
		if err != nil {
			return nil, fmt.Errorf("read service account file: %w", err)
		}
		credentials, err = google.CredentialsFromJSON(ctx, data, GCPAuthConfig.RequiredScopes...)
		if err != nil {
			return nil, fmt.Errorf("load credentials from file: %w", err)
		}
	} else if serviceAccountJSON != "" {
		// Use JSON string directly
		credentials, err = google.CredentialsFromJSON(ctx, []byte(serviceAccountJSON), GCPAuthConfig.RequiredScopes...)
		if err != nil {
			return nil, fmt.Errorf("load credentials from JSON: %w", err)
		}
	} else {
		// Try default credentials (for local dev)
		credentials, err = google.FindDefaultCredentials(ctx, GCPAuthConfig.RequiredScopes...)
		if err != nil {
			return nil, fmt.Errorf("find default credentials: %w", err)
		}
	}

	// Create Cloud Identity service
	cloudIdentityService, err := cloudidentity.NewService(ctx, option.WithCredentials(credentials))
	if err != nil {
		return nil, fmt.Errorf("create cloud identity service: %w", err)
	}

	// Create IAM service
	iamService, err := iam.NewService(ctx, option.WithCredentials(credentials))
	if err != nil {
		return nil, fmt.Errorf("create IAM service: %w", err)
	}

	// Create Resource Manager service
	resourceManagerService, err := cloudresourcemanager.NewService(ctx, option.WithCredentials(credentials))
	if err != nil {
		return nil, fmt.Errorf("create resource manager service: %w", err)
	}

	// Extract project ID from credentials if not provided
	if projectID == "" {
		// Try to get project ID from service account file
		if serviceAccountPath != "" {
			data, err := os.ReadFile(serviceAccountPath)
			if err == nil {
				var keyData map[string]interface{}
				if err := json.Unmarshal(data, &keyData); err == nil {
					if pid, ok := keyData["project_id"].(string); ok {
						projectID = pid
					}
				}
			}
		} else if serviceAccountJSON != "" {
			var keyData map[string]interface{}
			if err := json.Unmarshal([]byte(serviceAccountJSON), &keyData); err == nil {
				if pid, ok := keyData["project_id"].(string); ok {
					projectID = pid
				}
			}
		}
	}

	return &GCPClient{
		CloudIdentityService:  cloudIdentityService,
		IAMService:            iamService,
		ResourceManagerService: resourceManagerService,
		ProjectID:             projectID,
	}, nil
}

// ValidatePermissions checks if the service account has required permissions
func (c *GCPClient) ValidatePermissions(ctx context.Context) error {
	// Try to list groups (requires cloudidentity.groups.reader)
	_, err := c.CloudIdentityService.Groups.List().PageSize(1).Do()
	if err != nil {
		return fmt.Errorf("cloud identity API access denied: %w (need roles/cloudidentity.groups.reader)", err)
	}

	// Try to list IAM roles (requires iam.roles.list)
	_, err = c.IAMService.Projects.Roles.List("projects/" + c.ProjectID).Do()
	if err != nil {
		return fmt.Errorf("IAM API access denied: %w (need roles/iam.securityReviewer)", err)
	}

	// Try to get project (requires cloudresourcemanager.projects.get)
	_, err = c.ResourceManagerService.Projects.Get("projects/" + c.ProjectID).Do()
	if err != nil {
		return fmt.Errorf("resource manager API access denied: %w (need roles/viewer)", err)
	}

	return nil
}
