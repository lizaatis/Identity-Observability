package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ToxicComboRule defines a security rule pattern
type ToxicComboRule struct {
	Name     string `yaml:"name"`
	Query    string `yaml:"query"`
	Severity string `yaml:"severity"` // low, medium, high, critical
	Enabled  bool   `yaml:"enabled"`
}

// ToxicComboMatch represents a match found by a rule
type ToxicComboMatch struct {
	RuleName string                 `json:"rule_name"`
	Severity string                 `json:"severity"`
	Matches  []map[string]interface{} `json:"matches"`
	Count    int                    `json:"count"`
}

// ToxicComboEngine evaluates rules against the graph
type ToxicComboEngine struct {
	graphClient *GraphClient
	rules       []ToxicComboRule
}

// NewToxicComboEngine creates a new rule engine
func NewToxicComboEngine(graphClient *GraphClient, rulesPath string) (*ToxicComboEngine, error) {
	rules, err := loadRules(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	return &ToxicComboEngine{
		graphClient: graphClient,
		rules:       rules,
	}, nil
}

// EvaluateAll runs all enabled rules and returns matches
func (tce *ToxicComboEngine) EvaluateAll(ctx context.Context) ([]ToxicComboMatch, error) {
	var allMatches []ToxicComboMatch

	for _, rule := range tce.rules {
		if !rule.Enabled {
			continue
		}

		matches, err := tce.evaluateRule(ctx, rule)
		if err != nil {
			// Log error but continue with other rules
			continue
		}

		if len(matches) > 0 {
			allMatches = append(allMatches, ToxicComboMatch{
				RuleName: rule.Name,
				Severity: rule.Severity,
				Matches:  matches,
				Count:    len(matches),
			})
		}
	}

	return allMatches, nil
}

// EvaluateRule runs a single rule
func (tce *ToxicComboEngine) evaluateRule(ctx context.Context, rule ToxicComboRule) ([]map[string]interface{}, error) {
	records, err := tce.graphClient.ExecuteQuery(ctx, rule.Query, nil)
	if err != nil {
		return nil, fmt.Errorf("execute query for rule %s: %w", rule.Name, err)
	}

	return records, nil
}

// loadRules loads rules from YAML file(s)
func loadRules(rulesPath string) ([]ToxicComboRule, error) {
	if rulesPath == "" {
		rulesPath = "config/toxic_combo_rules.yaml"
	}

	// Check if file exists
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		// Return default rules
		return getDefaultRules(), nil
	}

	data, err := os.ReadFile(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("read rules file: %w", err)
	}

	var rules []ToxicComboRule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	return rules, nil
}

// getDefaultRules returns default toxic combo rules
func getDefaultRules() []ToxicComboRule {
	return []ToxicComboRule{
		{
			Name: "AWS Admin with no MFA",
			Query: `
				MATCH (u:User)-[:MEMBER_OF|HAS_ROLE*1..3]->(r:Role)
				WHERE r.admin = true 
				  AND r.source_system CONTAINS 'aws'
				  AND (u.mfa_enabled IS NULL OR u.mfa_enabled = false)
				RETURN u.id as identity_id, u.email, u.display_name, r.name as role_name, r.source_system
			`,
			Severity: "high",
			Enabled:  true,
		},
		{
			Name: "Cross-System Admin",
			Query: `
				MATCH (u:User)-[:HAS_ROLE|MEMBER_OF*1..3]->(r:Role)
				WHERE r.admin = true
				WITH u, COLLECT(DISTINCT r.source_system) as systems
				WHERE SIZE(systems) >= 2
				RETURN u.id as identity_id, u.email, u.display_name, systems
			`,
			Severity: "high",
			Enabled:  true,
		},
		{
			Name: "Orphaned Group with Admin Role",
			Query: `
				MATCH (g:Group)-[:HAS_ROLE]->(r:Role {admin: true})
				WHERE NOT (g)<-[:MEMBER_OF]-()
				RETURN g.id as group_id, g.name, r.name as role_name
			`,
			Severity: "medium",
			Enabled:  true,
		},
		{
			Name: "Privileged Role without MFA",
			Query: `
				MATCH (u:User)-[:HAS_ROLE|MEMBER_OF*1..3]->(r:Role)
				WHERE r.privilege_level IN ['admin', 'write']
				  AND (u.mfa_enabled IS NULL OR u.mfa_enabled = false)
				RETURN u.id as identity_id, u.email, r.name as role_name, r.privilege_level
			`,
			Severity: "high",
			Enabled:  true,
		},
	}
}

// SaveDefaultRules saves default rules to a YAML file
func SaveDefaultRules(rulesPath string) error {
	if rulesPath == "" {
		rulesPath = "config/toxic_combo_rules.yaml"
	}

	// Create config directory if it doesn't exist
	dir := filepath.Dir(rulesPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	rules := getDefaultRules()
	data, err := yaml.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}

	if err := os.WriteFile(rulesPath, data, 0644); err != nil {
		return fmt.Errorf("write rules file: %w", err)
	}

	return nil
}
