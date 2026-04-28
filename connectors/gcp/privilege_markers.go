package gcp

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

// PrivilegeMarker defines what counts as admin/elevated/toxic for a system
type PrivilegeMarker struct {
	System  string
	Level   PrivilegeLevel
	Markers []string
}

// PrivilegeMarkersConfig holds all privilege markers
type PrivilegeMarkersConfig struct {
	PrivilegedMarkers map[string]map[string][]string `yaml:"privileged_markers"`
}

var privilegeMarkersCache map[string]map[PrivilegeLevel][]string

// LoadPrivilegeMarkers loads privilege markers from YAML config
func LoadPrivilegeMarkers(configPath string) error {
	if configPath == "" {
		// Try multiple possible paths
		possiblePaths := []string{
			"config/privilege_markers.yaml",
			"../../config/privilege_markers.yaml",
			"../../../config/privilege_markers.yaml",
		}
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}
		if configPath == "" {
			// If not found, use default path and let it fail gracefully
			configPath = "config/privilege_markers.yaml"
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// If file doesn't exist, that's okay - we'll use defaults
		return nil
	}

	var config PrivilegeMarkersConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse privilege markers config: %w", err)
	}

	// Build cache
	privilegeMarkersCache = make(map[string]map[PrivilegeLevel][]string)
	
	for system, levels := range config.PrivilegedMarkers {
		privilegeMarkersCache[system] = make(map[PrivilegeLevel][]string)
		
		for levelStr, markers := range levels {
			level := PrivilegeLevel(levelStr)
			privilegeMarkersCache[system][level] = markers
		}
	}

	return nil
}

// IsPrivileged checks if a role/permission is privileged
func IsPrivileged(system, roleName string) bool {
	return GetPrivilegeLevel(system, roleName) == PrivilegeAdmin || 
		   GetPrivilegeLevel(system, roleName) == PrivilegeElevated
}

// IsAdmin checks if a role/permission is admin-level
func IsAdmin(system, roleName string) bool {
	return GetPrivilegeLevel(system, roleName) == PrivilegeAdmin
}

// IsToxic checks if a role/permission is toxic (admin without MFA, etc.)
func IsToxic(system, roleName string) bool {
	return GetPrivilegeLevel(system, roleName) == PrivilegeToxic
}

// GetPrivilegeLevel returns the privilege level for a role/permission
func GetPrivilegeLevel(system, roleName string) PrivilegeLevel {
	if privilegeMarkersCache == nil {
		// Try to load if not loaded
		_ = LoadPrivilegeMarkers("")
	}

	if systemMarkers, ok := privilegeMarkersCache[system]; ok {
		// Check in order: admin, elevated, toxic
		for _, level := range []PrivilegeLevel{PrivilegeAdmin, PrivilegeElevated, PrivilegeToxic} {
			if markers, ok := systemMarkers[level]; ok {
				for _, marker := range markers {
					// Exact match or contains (for ARNs)
					if roleName == marker || strings.Contains(roleName, marker) {
						return level
					}
				}
			}
		}
	}

	return PrivilegeStandard
}

// GetPrivilegeMarkers returns all markers for a system
func GetPrivilegeMarkers(system string) map[PrivilegeLevel][]string {
	if privilegeMarkersCache == nil {
		_ = LoadPrivilegeMarkers("")
	}

	if systemMarkers, ok := privilegeMarkersCache[system]; ok {
		return systemMarkers
	}

	return make(map[PrivilegeLevel][]string)
}
