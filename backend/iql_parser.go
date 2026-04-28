package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IQLQuery represents a parsed IQL query
type IQLQuery struct {
	Fields map[string]string // field:value pairs
	Raw    string            // Original query string
}

// IQLParser parses Identity Query Language queries
type IQLParser struct {
	pool        *pgxpool.Pool
	graphClient *GraphClient
}

// NewIQLParser creates a new IQL parser
func NewIQLParser(pool *pgxpool.Pool, graphClient *GraphClient) *IQLParser {
	return &IQLParser{
		pool:        pool,
		graphClient: graphClient,
	}
}

// Parse parses an IQL query string into structured query
func (p *IQLParser) Parse(query string) (*IQLQuery, error) {
	// Simple parser: field:value pairs separated by spaces
	// Examples:
	//   "system:aws mfa:false"
	//   "email:alice@company.com"
	//   "role:admin status:active"

	fields := make(map[string]string)
	parts := strings.Fields(query)

	for _, part := range parts {
		if idx := strings.Index(part, ":"); idx > 0 {
			field := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			if field != "" && value != "" {
				fields[field] = value
			}
		}
	}

	return &IQLQuery{
		Fields: fields,
		Raw:    query,
	}, nil
}

// Execute executes an IQL query and returns results
func (p *IQLParser) Execute(ctx context.Context, query *IQLQuery) ([]map[string]interface{}, error) {
	// Determine if we should use graph or relational query
	useGraph := false
	for field := range query.Fields {
		if field == "system" || field == "role" || field == "group" {
			useGraph = true
			break
		}
	}

	if useGraph {
		return p.executeGraphQuery(ctx, query)
	}
	return p.executeRelationalQuery(ctx, query)
}

// executeGraphQuery executes query against Neo4j
func (p *IQLParser) executeGraphQuery(ctx context.Context, query *IQLQuery) ([]map[string]interface{}, error) {
	var conditions []string
	params := make(map[string]interface{})

	// Build Cypher query
	cypher := "MATCH (u:User)"
	var returnClause = "RETURN u.id as identity_id, u.email, u.display_name, u.status"

	// Handle system filter
	if system, ok := query.Fields["system"]; ok {
		cypher += "-[:MEMBER_OF|HAS_ROLE*1..3]->(r:Role)"
		conditions = append(conditions, "r.source_system CONTAINS $system")
		params["system"] = system
		returnClause += ", r.name as role_name, r.source_system"
	}

	// Handle role filter
	if role, ok := query.Fields["role"]; ok {
		if !strings.Contains(cypher, "Role") {
			cypher += "-[:MEMBER_OF|HAS_ROLE*1..3]->(r:Role)"
		}
		conditions = append(conditions, "r.name CONTAINS $role OR r.name = $role")
		params["role"] = role
		if !strings.Contains(returnClause, "role_name") {
			returnClause += ", r.name as role_name"
		}
	}

	// Handle group filter
	if group, ok := query.Fields["group"]; ok {
		cypher += "-[:MEMBER_OF]->(g:Group)"
		conditions = append(conditions, "g.name CONTAINS $group OR g.name = $group")
		params["group"] = group
		returnClause += ", g.name as group_name"
	}

	// Handle MFA filter
	if mfa, ok := query.Fields["mfa"]; ok {
		if mfa == "false" || mfa == "no" {
			conditions = append(conditions, "(u.mfa_enabled IS NULL OR u.mfa_enabled = false)")
		} else if mfa == "true" || mfa == "yes" {
			conditions = append(conditions, "u.mfa_enabled = true")
		}
	}

	// Handle status filter
	if status, ok := query.Fields["status"]; ok {
		conditions = append(conditions, "u.status = $status")
		params["status"] = status
	}

	// Handle email filter
	if email, ok := query.Fields["email"]; ok {
		conditions = append(conditions, "u.email CONTAINS $email")
		params["email"] = email
	}

	// Build final query
	finalQuery := cypher
	if len(conditions) > 0 {
		finalQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	finalQuery += " " + returnClause + " LIMIT 100"

	records, err := p.graphClient.ExecuteQuery(ctx, finalQuery, params)
	if err != nil {
		return nil, fmt.Errorf("execute graph query: %w", err)
	}

	return records, nil
}

// executeRelationalQuery executes query against PostgreSQL
func (p *IQLParser) executeRelationalQuery(ctx context.Context, query *IQLQuery) ([]map[string]interface{}, error) {
	var conditions []string
	args := []interface{}{}
	argIdx := 1

	sql := "SELECT id, email, display_name, employee_id, status FROM identities WHERE 1=1"

	// Handle email filter
	if email, ok := query.Fields["email"]; ok {
		conditions = append(conditions, fmt.Sprintf("email ILIKE $%d", argIdx))
		args = append(args, "%"+email+"%")
		argIdx++
	}

	// Handle status filter
	if status, ok := query.Fields["status"]; ok {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	// Handle employee_id filter
	if empID, ok := query.Fields["employee_id"]; ok {
		conditions = append(conditions, fmt.Sprintf("employee_id = $%d", argIdx))
		args = append(args, empID)
		argIdx++
	}

	if len(conditions) > 0 {
		sql += " AND " + strings.Join(conditions, " AND ")
	}
	sql += " LIMIT 100"

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute relational query: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var email, displayName, employeeID, status string
		if err := rows.Scan(&id, &email, &displayName, &employeeID, &status); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"identity_id": id,
			"email":       email,
			"display_name": displayName,
			"employee_id": employeeID,
			"status":      status,
		})
	}

	return results, nil
}

// GetSupportedFields returns list of supported IQL fields
func (p *IQLParser) GetSupportedFields() []string {
	return []string{
		"system",      // Filter by source system (aws, okta, etc.)
		"role",        // Filter by role name
		"group",       // Filter by group name
		"mfa",         // Filter by MFA status (true/false)
		"status",      // Filter by identity status (active/inactive)
		"email",       // Filter by email (partial match)
		"employee_id", // Filter by employee ID
	}
}

// FormatQuery formats a query for display
func (p *IQLParser) FormatQuery(query *IQLQuery) string {
	var parts []string
	for field, value := range query.Fields {
		parts = append(parts, fmt.Sprintf("%s:%s", field, value))
	}
	return strings.Join(parts, " ")
}
