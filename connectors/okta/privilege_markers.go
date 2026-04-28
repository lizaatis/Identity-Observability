package okta

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PrivilegeLevel represents privilege levels
type PrivilegeLevel string

const (
	PrivilegeAdmin    PrivilegeLevel = "admin"
	PrivilegeElevated PrivilegeLevel = "elevated"
	PrivilegeStandard PrivilegeLevel = "standard"
	PrivilegeToxic    PrivilegeLevel = "toxic"
)

// PrivilegeMarkersConfig holds all privilege markers
type PrivilegeMarkersConfig struct {
	PrivilegedMarkers map[string]map[string][]string `yaml:"privileged_markers"`
}

var oktaPrivilegeMarkersCache map[PrivilegeLevel][]string

// LoadPrivilegeMarkers loads privilege markers from YAML config
func LoadPrivilegeMarkers(configPath string) error {
	if configPath == "" {
		configPath = "config/privilege_markers.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read privilege markers config: %w", err)
	}

	var config PrivilegeMarkersConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse privilege markers config: %w", err)
	}

	// Build cache for Okta
	if oktaMarkers, ok := config.PrivilegedMarkers["okta"]; ok {
		oktaPrivilegeMarkersCache = make(map[PrivilegeLevel][]string)
		
		for levelStr, markers := range oktaMarkers {
			level := PrivilegeLevel(levelStr)
			oktaPrivilegeMarkersCache[level] = markers
		}
	}

	return nil
}

// IsPrivileged checks if an Okta role is privileged
func IsPrivileged(roleName string) bool {
	return GetPrivilegeLevel(roleName) == PrivilegeAdmin || 
		   GetPrivilegeLevel(roleName) == PrivilegeElevated
}

// IsAdmin checks if an Okta role is admin-level
func IsAdmin(roleName string) bool {
	return GetPrivilegeLevel(roleName) == PrivilegeAdmin
}

// IsToxic checks if an Okta role is toxic (admin without MFA, etc.)
func IsToxic(roleName string) bool {
	return GetPrivilegeLevel(roleName) == PrivilegeToxic
}

// GetPrivilegeLevel returns the privilege level for an Okta role
func GetPrivilegeLevel(roleName string) PrivilegeLevel {
	if oktaPrivilegeMarkersCache == nil {
		// Try to load if not loaded
		_ = LoadPrivilegeMarkers("")
	}

	if oktaPrivilegeMarkersCache != nil {
		// Check in order: admin, elevated, toxic
		for _, level := range []PrivilegeLevel{PrivilegeAdmin, PrivilegeElevated, PrivilegeToxic} {
			if markers, ok := oktaPrivilegeMarkersCache[level]; ok {
				for _, marker := range markers {
					// Exact match or contains
					if roleName == marker || strings.Contains(roleName, marker) {
						return level
					}
				}
			}
		}
	}

	return PrivilegeStandard
}
