package sailpoint

import (
	"fmt"
	"os"
	"time"
)

// Config holds SailPoint connector configuration
type Config struct {
	// Database
	DatabaseURL string

	// SailPoint API (IdentityNow)
	Tenant       string // Can be just tenant name or full URL
	ClientID     string
	ClientSecret string
	BaseURL      string // Optional: custom base URL (e.g., identitynow-demo.com)

	// Sync settings
	SourceSystem    string // e.g. "sailpoint_prod"
	ConnectorName   string // e.g. "sailpoint_connector"
	IncrementalSync bool
	ChangedSince    *time.Time // For incremental syncs

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
		Tenant:             getEnv("SAILPOINT_TENANT", ""),
		ClientID:           getEnv("SAILPOINT_CLIENT_ID", ""),
		ClientSecret:       getEnv("SAILPOINT_CLIENT_SECRET", ""),
		BaseURL:            getEnv("SAILPOINT_BASE_URL", "identitynow-demo.com"),
		SourceSystem:       getEnv("SAILPOINT_SOURCE_SYSTEM", "sailpoint_prod"),
		ConnectorName:      getEnv("SAILPOINT_CONNECTOR_NAME", "sailpoint_connector"),
		IncrementalSync:    getEnvBool("SAILPOINT_INCREMENTAL_SYNC", false),
		MaxRetries:         getEnvInt("SAILPOINT_MAX_RETRIES", 3),
		RetryBackoff:       getEnvDuration("SAILPOINT_RETRY_BACKOFF", 2*time.Second),
		RateLimitWindow:    getEnvDuration("SAILPOINT_RATE_LIMIT_WINDOW", 1*time.Minute),
		MinConfidenceScore: getEnvFloat("SAILPOINT_MIN_CONFIDENCE", 0.8),
	}

	// Parse changed_since if provided
	if sinceStr := os.Getenv("SAILPOINT_CHANGED_SINCE"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			cfg.ChangedSince = &t
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
