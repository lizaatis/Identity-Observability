package gcp

import (
	"fmt"
	"os"
	"time"
)

// Config holds GCP connector configuration
type Config struct {
	// Database
	DatabaseURL string

	// GCP Service Account
	ServiceAccountPath string // Path to service account JSON file
	ServiceAccountJSON string // Or JSON content directly

	// GCP Project
	ProjectID string

	// Sync settings
	SourceSystem    string // e.g. "gcp_prod"
	ConnectorName   string // e.g. "gcp_connector"
	IncrementalSync bool
	ChangedSince    *time.Time // For incremental syncs

	// Organization/Resource scope
	OrganizationID string // Optional: GCP organization ID
	FolderIDs      []string // Optional: Specific folder IDs to scan

	// Rate limiting
	MaxRetries      int
	RetryBackoff    time.Duration
	RateLimitWindow time.Duration

	// Identity resolution
	MinConfidenceScore float64 // Minimum confidence for auto-merge (0.0-1.0)
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	cfg := &Config{
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		ServiceAccountPath: getEnv("GCP_SERVICE_ACCOUNT_PATH", ""),
		ServiceAccountJSON: getEnv("GCP_SERVICE_ACCOUNT_JSON", ""),
		ProjectID:           getEnv("GCP_PROJECT_ID", ""),
		SourceSystem:       getEnv("GCP_SOURCE_SYSTEM", "gcp_prod"),
		ConnectorName:      getEnv("GCP_CONNECTOR_NAME", "gcp_connector"),
		IncrementalSync:    getEnvBool("GCP_INCREMENTAL_SYNC", false),
		MaxRetries:         getEnvInt("GCP_MAX_RETRIES", 3),
		RetryBackoff:       getEnvDuration("GCP_RETRY_BACKOFF", 2*time.Second),
		RateLimitWindow:    getEnvDuration("GCP_RATE_LIMIT_WINDOW", 1*time.Minute),
		MinConfidenceScore: getEnvFloat("GCP_MIN_CONFIDENCE", 0.8),
		OrganizationID:     getEnv("GCP_ORGANIZATION_ID", ""),
	}

	// Parse changed_since if provided
	if sinceStr := os.Getenv("GCP_CHANGED_SINCE"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			cfg.ChangedSince = &t
		}
	}

	// Parse folder IDs (comma-separated)
	if folderStr := os.Getenv("GCP_FOLDER_IDS"); folderStr != "" {
		cfg.FolderIDs = []string{}
		// Simple split - can be enhanced
		for _, f := range splitString(folderStr, ",") {
			if f != "" {
				cfg.FolderIDs = append(cfg.FolderIDs, f)
			}
		}
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		var result float64
		if _, err := fmt.Sscanf(value, "%f", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func splitString(s, sep string) []string {
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}
