package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lenses: cross-system discovery lists (privileged, cross-cloud admins, deadends, no-MFA).
// GET /api/v1/lenses/privileged?source_system=okta_mock&limit=50
// GET /api/v1/lenses/cross-cloud-admins?limit=50
// GET /api/v1/lenses/deadends?limit=50
// GET /api/v1/lenses/no-mfa?limit=50

func GetLensPrivileged(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		listLens(c, pool, "privileged", `
			SELECT DISTINCT i.id, i.email, i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score, rs.max_severity
			FROM identities i
			JOIN identity_effective_permissions iep ON iep.identity_id = i.id
			LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(iep.role_name) LIKE '%admin%' OR LOWER(iep.role_name) LIKE '%privileged%'
			   OR LOWER(iep.role_name) LIKE '%root%' OR LOWER(iep.role_name) LIKE '%super%'
			   OR iep.privilege_level IN ('admin', 'administrator', 'super_admin'))
			{filter}
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC NULLS LAST
			LIMIT $1
		`, c.Query("source_system"), c.DefaultQuery("limit", "100"))
	}
}

func GetLensCrossCloudAdmins(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		listLens(c, pool, "cross_cloud_admins", `
			SELECT DISTINCT i.id, i.email, i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score, rs.max_severity
			FROM identities i
			JOIN identity_sources is2 ON is2.identity_id = i.id
			JOIN identity_role ir ON ir.identity_id = i.id
			JOIN roles r ON r.id = ir.role_id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(r.name) LIKE '%admin%' OR LOWER(r.name) LIKE '%privileged%' OR LOWER(r.name) LIKE '%root%')
			{filter}
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			HAVING COUNT(DISTINCT is2.source_system) > 1
			ORDER BY rs.score DESC NULLS LAST
			LIMIT $1
		`, "", c.DefaultQuery("limit", "100"))
	}
}

func GetLensDeadends(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		listLensWithReason(c, pool, "deadends", c.DefaultQuery("limit", "100"))
	}
}

func GetLensNoMfa(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		listLens(c, pool, "no_mfa", `
			SELECT DISTINCT i.id, i.email, i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score, rs.max_severity
			FROM identities i
			JOIN identity_role ir ON ir.identity_id = i.id
			JOIN roles r ON r.id = ir.role_id
			LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(r.name) LIKE '%admin%' OR LOWER(r.name) LIKE '%privileged%'
			   OR LOWER(r.name) LIKE '%root%' OR LOWER(r.name) LIKE '%super%')
			  AND NOT EXISTS (
				SELECT 1 FROM identity_sources is3
				WHERE is3.identity_id = i.id
				  AND (is3.metadata->>'mfa_enabled' = 'true' OR is3.metadata->>'mfa_configured' = 'true')
			  )
			{filter}
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC NULLS LAST
			LIMIT $1
		`, c.Query("source_system"), c.DefaultQuery("limit", "100"))
	}
}

func listLens(c *gin.Context, pool *pgxpool.Pool, lensName, baseQuery string, sourceSystem, limitStr string) {
	ctx := c.Request.Context()
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter := ""
	args := []interface{}{limit}
	if sourceSystem != "" {
		filter = " AND EXISTS (SELECT 1 FROM identity_sources isx WHERE isx.identity_id = i.id AND isx.source_system = $2)"
		args = append(args, sourceSystem)
	}
	q := strings.Replace(baseQuery, "{filter}", filter, 1)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		// If the underlying view/table isn't available (e.g. migrations not run),
		// return an empty lens instead of a 500 so the UI can still render.
		c.JSON(http.StatusOK, gin.H{
			"lens":  lensName,
			"count": 0,
			"items": []IdentitySummary{},
			"error": err.Error(),
		})
		return
	}
	defer rows.Close()
	items := scanIdentitySummaries(rows)
	c.JSON(http.StatusOK, gin.H{"lens": lensName, "count": len(items), "items": items})
}

func listLensWithReason(c *gin.Context, pool *pgxpool.Pool, lensName, limitStr string) {
	ctx := c.Request.Context()
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT i.id, i.email, i.display_name,
			ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
			rs.score, rs.max_severity,
			(SELECT rf.message FROM risk_flags rf WHERE rf.identity_id = i.id AND rf.is_deadend = true AND rf.cleared_at IS NULL ORDER BY rf.created_at DESC LIMIT 1) as deadend_reason
		FROM identities i
		JOIN risk_flags rf ON rf.identity_id = i.id
		LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
		LEFT JOIN risk_scores rs ON rs.identity_id = i.id
		WHERE rf.is_deadend = true AND rf.cleared_at IS NULL
		GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
		ORDER BY rs.score DESC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		// Same behavior as other lenses: surface as empty list, not 500.
		c.JSON(http.StatusOK, gin.H{
			"lens":  lensName,
			"count": 0,
			"items": []IdentitySummary{},
			"error": err.Error(),
		})
		return
	}
	defer rows.Close()
	items := scanIdentitySummariesWithReason(rows)
	c.JSON(http.StatusOK, gin.H{"lens": lensName, "count": len(items), "items": items})
}

type rowsScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
}

func scanIdentitySummaries(rows rowsScanner) []IdentitySummary {
	var items []IdentitySummary
	for rows.Next() {
		var ident IdentitySummary
		var sourceSystems []string
		var riskScore *int
		var maxSeverity *string
		if rows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity) != nil {
			continue
		}
		if sourceSystems == nil {
			sourceSystems = []string{}
		}
		ident.SourceSystems = sourceSystems
		ident.RiskScore = riskScore
		ident.MaxSeverity = maxSeverity
		items = append(items, ident)
	}
	return items
}

func scanIdentitySummariesWithReason(rows rowsScanner) []IdentitySummary {
	var items []IdentitySummary
	for rows.Next() {
		var ident IdentitySummary
		var sourceSystems []string
		var riskScore *int
		var maxSeverity *string
		var deadendReason *string
		if rows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity, &deadendReason) != nil {
			continue
		}
		if sourceSystems == nil {
			sourceSystems = []string{}
		}
		ident.SourceSystems = sourceSystems
		ident.RiskScore = riskScore
		ident.MaxSeverity = maxSeverity
		ident.DeadendReason = deadendReason
		items = append(items, ident)
	}
	return items
}
