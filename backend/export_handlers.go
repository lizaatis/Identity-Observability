package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Export identity as CSV or PDF
func exportIdentity(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}

		format := c.Query("format")
		if format != "csv" && format != "pdf" {
			format = "csv" // default
		}

		ctx := c.Request.Context()

		// Get identity data
		var ident IdentityDTO
		err = pool.QueryRow(ctx, `
			SELECT id, employee_id, email, display_name, status, created_at::text, updated_at::text
			FROM identities WHERE id = $1`, id,
		).Scan(&ident.ID, &ident.EmployeeID, &ident.Email, &ident.DisplayName, &ident.Status, &ident.CreatedAt, &ident.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "identity not found"})
			return
		}

		// Get risk score
		var riskScore int
		var maxSeverity string
		_ = pool.QueryRow(ctx, `
			SELECT score, max_severity FROM risk_scores WHERE identity_id = $1
		`, id).Scan(&riskScore, &maxSeverity)

		// Get effective permissions
		rows, _ := pool.Query(ctx, `
			SELECT permission_name, resource_type, role_name, path_type, group_name
			FROM identity_effective_permissions WHERE identity_id = $1
		`, id)
		defer rows.Close()

		var permissions []map[string]interface{}
		for rows.Next() {
			var permName, resourceType, roleName, pathType string
			var groupName *string
			rows.Scan(&permName, &resourceType, &roleName, &pathType, &groupName)
			permissions = append(permissions, map[string]interface{}{
				"permission": permName,
				"resource":  resourceType,
				"role":      roleName,
				"path":      pathType,
				"group":     groupName,
			})
		}

		// Get risk flags
		flagRows, _ := pool.Query(ctx, `
			SELECT rule_key, severity, is_deadend, message
			FROM risk_flags WHERE identity_id = $1 AND cleared_at IS NULL
		`, id)
		defer flagRows.Close()

		var flags []map[string]interface{}
		for flagRows.Next() {
			var ruleKey, severity, message string
			var isDeadend bool
			flagRows.Scan(&ruleKey, &severity, &isDeadend, &message)
			flags = append(flags, map[string]interface{}{
				"rule":     ruleKey,
				"severity": severity,
				"deadend":  isDeadend,
				"message":  message,
			})
		}

		if format == "csv" {
			exportIdentityCSV(c, ident, riskScore, maxSeverity, permissions, flags)
		} else {
			exportIdentityPDF(c, ident, riskScore, maxSeverity, permissions, flags)
		}
	}
}

// Export high-risk identities
func exportHighRisk(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		format := c.Query("format")
		if format != "csv" && format != "pdf" {
			format = "csv"
		}

		ctx := c.Request.Context()

		rows, _ := pool.Query(ctx, `
			SELECT 
				i.id, i.email, i.display_name, i.employee_id,
				rs.score, rs.max_severity,
				COUNT(rf.id) as flag_count
			FROM identities i
			JOIN risk_scores rs ON rs.identity_id = i.id
			LEFT JOIN risk_flags rf ON rf.identity_id = i.id AND rf.cleared_at IS NULL
			WHERE rs.score >= 40
			GROUP BY i.id, i.email, i.display_name, i.employee_id, rs.score, rs.max_severity
			ORDER BY rs.score DESC
		`)
		defer rows.Close()

		var identities []map[string]interface{}
		for rows.Next() {
			var id int64
			var email, displayName, employeeID, maxSeverity string
			var score int
			var flagCount int64
			rows.Scan(&id, &email, &displayName, &employeeID, &score, &maxSeverity, &flagCount)
			identities = append(identities, map[string]interface{}{
				"id":          id,
				"email":       email,
				"displayName": displayName,
				"employeeID":  employeeID,
				"score":       score,
				"severity":    maxSeverity,
				"flagCount":   flagCount,
			})
		}

		if format == "csv" {
			exportHighRiskCSV(c, identities)
		} else {
			exportHighRiskPDF(c, identities)
		}
	}
}

// Export deadend identities
func exportDeadends(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		format := c.Query("format")
		if format != "csv" && format != "pdf" {
			format = "csv"
		}

		ctx := c.Request.Context()

		rows, _ := pool.Query(ctx, `
			SELECT DISTINCT
				i.id, i.email, i.display_name, i.employee_id,
				rf.rule_key, rf.severity, rf.message
			FROM identities i
			JOIN risk_flags rf ON rf.identity_id = i.id
			WHERE rf.is_deadend = true AND rf.cleared_at IS NULL
			ORDER BY i.id, rf.severity DESC
		`)
		defer rows.Close()

		var deadends []map[string]interface{}
		for rows.Next() {
			var id int64
			var email, displayName, employeeID, ruleKey, severity, message string
			rows.Scan(&id, &email, &displayName, &employeeID, &ruleKey, &severity, &message)
			deadends = append(deadends, map[string]interface{}{
				"id":          id,
				"email":       email,
				"displayName": displayName,
				"employeeID":  employeeID,
				"rule":        ruleKey,
				"severity":    severity,
				"message":     message,
			})
		}

		if format == "csv" {
			exportDeadendsCSV(c, deadends)
		} else {
			exportDeadendsPDF(c, deadends)
		}
	}
}

// CSV export helpers
func exportIdentityCSV(c *gin.Context, ident IdentityDTO, score int, severity string, perms, flags []map[string]interface{}) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=identity-%d.csv", ident.ID))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	writer.Write([]string{"Identity Export"})
	writer.Write([]string{"ID", strconv.FormatInt(ident.ID, 10)})
	writer.Write([]string{"Email", ident.Email})
	writer.Write([]string{"Display Name", getString(ident.DisplayName)})
	writer.Write([]string{"Employee ID", getString(ident.EmployeeID)})
	writer.Write([]string{"Status", ident.Status})
	writer.Write([]string{"Risk Score", strconv.Itoa(score)})
	writer.Write([]string{"Max Severity", severity})
	writer.Write([]string{""})

	writer.Write([]string{"Effective Permissions"})
	writer.Write([]string{"Permission", "Resource Type", "Role", "Path Type", "Group"})
	for _, p := range perms {
		writer.Write([]string{
			getString(p["permission"]),
			getString(p["resource"]),
			getString(p["role"]),
			getString(p["path"]),
			getString(p["group"]),
		})
	}
	writer.Write([]string{""})

	writer.Write([]string{"Risk Flags"})
	writer.Write([]string{"Rule", "Severity", "Deadend", "Message"})
	for _, f := range flags {
		writer.Write([]string{
			getString(f["rule"]),
			getString(f["severity"]),
			fmt.Sprintf("%v", f["deadend"]),
			getString(f["message"]),
		})
	}
}

func exportHighRiskCSV(c *gin.Context, identities []map[string]interface{}) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=high-risk-identities.csv")

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	writer.Write([]string{"ID", "Email", "Display Name", "Employee ID", "Risk Score", "Severity", "Flag Count"})
	for _, i := range identities {
		writer.Write([]string{
			strconv.FormatInt(i["id"].(int64), 10),
			getString(i["email"]),
			getString(i["displayName"]),
			getString(i["employeeID"]),
			strconv.Itoa(i["score"].(int)),
			getString(i["severity"]),
			strconv.FormatInt(i["flagCount"].(int64), 10),
		})
	}
}

func exportDeadendsCSV(c *gin.Context, deadends []map[string]interface{}) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=deadend-identities.csv")

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	writer.Write([]string{"ID", "Email", "Display Name", "Employee ID", "Rule", "Severity", "Message"})
	for _, d := range deadends {
		writer.Write([]string{
			strconv.FormatInt(d["id"].(int64), 10),
			getString(d["email"]),
			getString(d["displayName"]),
			getString(d["employeeID"]),
			getString(d["rule"]),
			getString(d["severity"]),
			getString(d["message"]),
		})
	}
}

// PDF export helpers
// Note: PDF export requires jspdf library. For now, we'll return a simple text-based PDF
// In production, install: go get github.com/jspdf/jspdf
func exportIdentityPDF(c *gin.Context, ident IdentityDTO, score int, severity string, perms, flags []map[string]interface{}) {
	// Simple PDF-like text export (replace with actual PDF library in production)
	pdfContent := fmt.Sprintf(`Identity Export

ID: %d
Email: %s
Display Name: %s
Risk Score: %d (%s)

Effective Permissions:
%s

Risk Flags:
%s
`,
		ident.ID, ident.Email, getString(ident.DisplayName), score, severity,
		formatPermissions(perms),
		formatFlags(flags),
	)

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=identity-%d.pdf", ident.ID))
	c.String(http.StatusOK, pdfContent)
}

func exportHighRiskPDF(c *gin.Context, identities []map[string]interface{}) {
	content := "High Risk Identities\n\n"
	for _, i := range identities {
		content += fmt.Sprintf("ID: %d, Email: %s, Score: %d, Severity: %s\n",
			i["id"], getString(i["email"]), i["score"], getString(i["severity"]))
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename=high-risk-identities.pdf")
	c.String(http.StatusOK, content)
}

func exportDeadendsPDF(c *gin.Context, deadends []map[string]interface{}) {
	content := "Deadend Identities\n\n"
	for _, d := range deadends {
		content += fmt.Sprintf("ID: %d, Email: %s, Rule: %s, Severity: %s\n",
			d["id"], getString(d["email"]), getString(d["rule"]), getString(d["severity"]))
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename=deadend-identities.pdf")
	c.String(http.StatusOK, content)
}

func formatPermissions(perms []map[string]interface{}) string {
	result := "Permission | Resource | Role | Path | Group\n"
	for _, p := range perms {
		result += fmt.Sprintf("%s | %s | %s | %s | %s\n",
			getString(p["permission"]), getString(p["resource"]),
			getString(p["role"]), getString(p["path"]), getString(p["group"]))
	}
	return result
}

func formatFlags(flags []map[string]interface{}) string {
	result := "Rule | Severity | Deadend | Message\n"
	for _, f := range flags {
		result += fmt.Sprintf("%s | %s | %v | %s\n",
			getString(f["rule"]), getString(f["severity"]), f["deadend"], getString(f["message"]))
	}
	return result
}

func getString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(*string); ok {
		if s == nil {
			return ""
		}
		return *s
	}
	return fmt.Sprintf("%v", v)
}
