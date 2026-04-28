package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// CustomRule represents a user-defined rule
type CustomRule struct {
	ID           int64                  `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	RuleYAML     string                 `json:"rule_yaml"`
	Severity     string                 `json:"severity"`
	Enabled      bool                   `json:"enabled"`
	CreatedBy    string                 `json:"created_by"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
	LastTestedAt *string                `json:"last_tested_at,omitempty"`
	TestResults  map[string]interface{} `json:"test_results,omitempty"`
}

// CreateCustomRule creates a new custom rule
func CreateCustomRule(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			RuleYAML    string `json:"rule_yaml" binding:"required"`
			Severity    string `json:"severity"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}

		// Validate YAML
		var rule ToxicComboRule
		if err := yaml.Unmarshal([]byte(req.RuleYAML), &rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid YAML", "details": err.Error()})
			return
		}

		createdBy := c.GetString("user_email")
		if createdBy == "" {
			createdBy = "system"
		}

		if req.Severity == "" {
			req.Severity = "medium"
		}

		ctx := c.Request.Context()
		var ruleID int64
		err := pool.QueryRow(ctx, `
			INSERT INTO custom_rules (name, description, rule_yaml, severity, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, req.Name, req.Description, req.RuleYAML, req.Severity, createdBy).Scan(&ruleID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create rule"})
			return
		}

		// Test the rule if graph client is available
		if graphClient != nil {
			go testCustomRule(context.Background(), pool, graphClient, ruleID, rule)
		}

		c.JSON(http.StatusCreated, gin.H{"id": ruleID, "status": "created"})
	}
}

// TestCustomRule tests a custom rule
func TestCustomRule(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		ruleID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || ruleID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule id"})
			return
		}

		ctx := c.Request.Context()

		// Get rule
		var ruleYAML string
		err = pool.QueryRow(ctx, `SELECT rule_yaml FROM custom_rules WHERE id = $1`, ruleID).Scan(&ruleYAML)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
			return
		}

		// Parse YAML
		var rule ToxicComboRule
		if err := yaml.Unmarshal([]byte(ruleYAML), &rule); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule YAML"})
			return
		}

		// Execute rule
		engine := &ToxicComboEngine{
			graphClient: graphClient,
			rules:       []ToxicComboRule{rule},
		}

		matches, err := engine.EvaluateAll(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "rule execution failed", "details": err.Error()})
			return
		}

		// Store test results
		testResults := map[string]interface{}{
			"match_count": len(matches),
			"matches":     matches,
			"tested_at":   time.Now().Format(time.RFC3339),
		}

		testResultsJSON, _ := json.Marshal(testResults)
		pool.Exec(ctx, `
			UPDATE custom_rules
			SET last_tested_at = NOW(),
			    test_results = $1
			WHERE id = $2
		`, testResultsJSON, ruleID)

		c.JSON(http.StatusOK, gin.H{
			"rule_id":    ruleID,
			"match_count": len(matches),
			"matches":    matches,
		})
	}
}

// ListCustomRules lists all custom rules
func ListCustomRules(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		rows, err := pool.Query(ctx, `
			SELECT id, name, description, rule_yaml, severity, enabled,
			       created_by, created_at::text, updated_at::text,
			       last_tested_at::text, test_results
			FROM custom_rules
			ORDER BY created_at DESC
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list rules"})
			return
		}
		defer rows.Close()

		var rules []CustomRule
		for rows.Next() {
			var rule CustomRule
			var testResultsJSON []byte
			err := rows.Scan(
				&rule.ID, &rule.Name, &rule.Description, &rule.RuleYAML,
				&rule.Severity, &rule.Enabled, &rule.CreatedBy,
				&rule.CreatedAt, &rule.UpdatedAt, &rule.LastTestedAt, &testResultsJSON,
			)
			if err != nil {
				continue
			}

			if testResultsJSON != nil {
				json.Unmarshal(testResultsJSON, &rule.TestResults)
			}

			rules = append(rules, rule)
		}

		c.JSON(http.StatusOK, gin.H{"rules": rules})
	}
}

// testCustomRule tests a rule in the background
func testCustomRule(ctx context.Context, pool *pgxpool.Pool, graphClient *GraphClient, ruleID int64, rule ToxicComboRule) {
	engine := &ToxicComboEngine{
		graphClient: graphClient,
		rules:       []ToxicComboRule{rule},
	}

	matches, err := engine.EvaluateAll(ctx)
	if err != nil {
		return
	}

	testResults := map[string]interface{}{
		"match_count": len(matches),
		"matches":     matches,
		"tested_at":   time.Now().Format(time.RFC3339),
	}

	testResultsJSON, _ := json.Marshal(testResults)
	pool.Exec(ctx, `
		UPDATE custom_rules
		SET last_tested_at = NOW(),
		    test_results = $1
		WHERE id = $2
	`, testResultsJSON, ruleID)
}
