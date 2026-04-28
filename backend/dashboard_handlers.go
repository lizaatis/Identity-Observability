package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardStatsResponse for GET /api/v1/dashboard/stats
type DashboardStatsResponse struct {
	TotalIdentities      int                        `json:"total_identities"`
	PrivilegedCount      int                        `json:"privileged_count"`
	CrossCloudAdmins     int                        `json:"cross_cloud_admins"`
	MfaCoverage          float64                    `json:"mfa_coverage"`
	DeadendCount         int                        `json:"deadend_count"`
	IdentitiesBySystem   map[string]int              `json:"identities_by_system"`
	RiskDistribution     map[string]int              `json:"risk_distribution"`
	RecentSyncs          []SyncSummaryDTO            `json:"recent_syncs"`
	TopRiskIssues        []RiskIssueSummary          `json:"top_risk_issues"`
	NoMfaIdentities      []IdentitySummary           `json:"no_mfa_identities"`
	DeadendIdentities    []IdentitySummary           `json:"deadend_identities"`
	CrossCloudAdminList  []IdentitySummary           `json:"cross_cloud_admin_list"`
	PrivilegedIdentities []IdentitySummary           `json:"privileged_identities"`
}

type IdentitySummary struct {
	IdentityID    int64    `json:"identity_id"`
	Email         string   `json:"email"`
	DisplayName   *string  `json:"display_name,omitempty"`
	SourceSystems []string `json:"source_systems"`
	RiskScore     *int     `json:"risk_score,omitempty"`
	MaxSeverity   *string  `json:"max_severity,omitempty"`
	DeadendReason *string  `json:"deadend_reason,omitempty"` // why this identity is a deadend
}

type SyncSummaryDTO struct {
	SourceSystem  string `json:"source_system"`
	ConnectorName string `json:"connector_name"`
	LastSyncAt    string `json:"last_sync_at"`
	Status        string `json:"status"`
	IdentityCount int    `json:"identity_count"`
}

type RiskIssueSummary struct {
	RuleKey   string `json:"rule_key"`
	Severity  string `json:"severity"`
	Count     int    `json:"count"`
	IsDeadend bool   `json:"is_deadend"`
	Message   string `json:"message"`
}

// Get dashboard statistics
func getDashboardStats(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		stats := DashboardStatsResponse{
			IdentitiesBySystem: make(map[string]int),
			RiskDistribution:   make(map[string]int),
			RecentSyncs:        []SyncSummaryDTO{},
			TopRiskIssues:      []RiskIssueSummary{},
			NoMfaIdentities:    []IdentitySummary{},
			DeadendIdentities:  []IdentitySummary{},
			CrossCloudAdminList: []IdentitySummary{},
			PrivilegedIdentities: []IdentitySummary{},
		}

		// Total identities
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM identities`).Scan(&stats.TotalIdentities); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get total identities", "details": err.Error()})
			return
		}

		// Identities by source system
		rows, _ := pool.Query(ctx, `
			SELECT source_system, COUNT(DISTINCT identity_id) as count
			FROM identity_sources
			GROUP BY source_system
		`)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var system string
				var count int
				if rows.Scan(&system, &count) == nil {
					stats.IdentitiesBySystem[system] = count
				}
			}
		}

		// Privileged identities (those with admin roles via direct assignment OR via groups)
		// This includes identities with admin roles directly OR through group memberships
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT i.id)
			FROM identities i
			WHERE EXISTS (
				-- Direct role assignment
				SELECT 1
				FROM identity_role ir
				JOIN roles r ON r.id = ir.role_id
				WHERE ir.identity_id = i.id
				  AND (LOWER(r.name) LIKE '%admin%' 
				   OR LOWER(r.name) LIKE '%privileged%'
				   OR LOWER(r.name) LIKE '%root%'
				   OR LOWER(r.name) LIKE '%super%'
				   OR r.privilege_level IN ('admin', 'administrator', 'super_admin'))
			) OR EXISTS (
				-- Via group membership
				SELECT 1
				FROM identity_group ig
				JOIN group_role gr ON gr.group_id = ig.group_id
				JOIN roles r ON r.id = gr.role_id
				WHERE ig.identity_id = i.id
				  AND (LOWER(r.name) LIKE '%admin%' 
				   OR LOWER(r.name) LIKE '%privileged%'
				   OR LOWER(r.name) LIKE '%root%'
				   OR LOWER(r.name) LIKE '%super%'
				   OR r.privilege_level IN ('admin', 'administrator', 'super_admin'))
			) OR EXISTS (
				-- Via effective permissions (includes all paths)
				SELECT 1
				FROM identity_effective_permissions iep
				WHERE iep.identity_id = i.id
				  AND (LOWER(iep.role_name) LIKE '%admin%' 
				   OR LOWER(iep.role_name) LIKE '%privileged%'
				   OR LOWER(iep.role_name) LIKE '%root%'
				   OR LOWER(iep.role_name) LIKE '%super%'
				   OR iep.privilege_level IN ('admin', 'administrator', 'super_admin'))
			)
		`).Scan(&stats.PrivilegedCount); err != nil {
			// If query fails (e.g., view doesn't exist), set to 0
			stats.PrivilegedCount = 0
		}

		// Cross-cloud admins (admin in multiple systems)
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT i.id)
			FROM identities i
			WHERE (
				SELECT COUNT(DISTINCT is2.source_system)
				FROM identity_sources is2
				JOIN identity_role ir2 ON ir2.identity_id = is2.identity_id
				JOIN roles r2 ON r2.id = ir2.role_id
				WHERE is2.identity_id = i.id
				  AND (LOWER(r2.name) LIKE '%admin%' 
				   OR LOWER(r2.name) LIKE '%privileged%'
				   OR LOWER(r2.name) LIKE '%root%')
			) > 1
		`).Scan(&stats.CrossCloudAdmins); err != nil {
			stats.CrossCloudAdmins = 0
		}

		// MFA Coverage (from identity_sources metadata)
		// For now, if no MFA data is available, we'll calculate based on privileged identities
		// In production, connectors should sync MFA status
		var totalWithMFA, totalChecked int
		pool.QueryRow(ctx, `
			SELECT 
				COUNT(*) FILTER (WHERE metadata->>'mfa_enabled' = 'true' OR metadata->>'mfa_configured' = 'true') as with_mfa,
				COUNT(*) FILTER (WHERE metadata ? 'mfa_enabled' OR metadata ? 'mfa_configured') as checked
			FROM identity_sources
		`).Scan(&totalWithMFA, &totalChecked)
		
		if totalChecked > 0 {
			stats.MfaCoverage = float64(totalWithMFA) / float64(totalChecked) * 100
		} else {
			// If no MFA metadata exists, estimate based on privileged identities
			// This is a fallback - connectors should sync MFA data
			var privilegedCount, privilegedWithMFA int
			pool.QueryRow(ctx, `
				SELECT 
					COUNT(DISTINCT i.id) as total_priv,
					COUNT(DISTINCT i.id) FILTER (
						WHERE EXISTS (
							SELECT 1 FROM identity_sources is2
							WHERE is2.identity_id = i.id
							  AND (is2.metadata->>'mfa_enabled' = 'true' OR is2.metadata->>'mfa_configured' = 'true')
						)
					) as priv_with_mfa
				FROM identities i
				WHERE EXISTS (
					SELECT 1 FROM identity_role ir
					JOIN roles r ON r.id = ir.role_id
					WHERE ir.identity_id = i.id
					  AND (LOWER(r.name) LIKE '%admin%' 
					   OR LOWER(r.name) LIKE '%privileged%'
					   OR LOWER(r.name) LIKE '%root%'
					   OR LOWER(r.name) LIKE '%super%')
				)
			`).Scan(&privilegedCount, &privilegedWithMFA)
			
			if privilegedCount > 0 {
				// Estimate: if we can't determine MFA, assume 0% for now
				stats.MfaCoverage = 0
			} else {
				stats.MfaCoverage = 0
			}
		}

		// Deadend count (identities with deadend flags)
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(DISTINCT identity_id)
			FROM risk_flags
			WHERE is_deadend = true AND cleared_at IS NULL
		`).Scan(&stats.DeadendCount); err != nil {
			stats.DeadendCount = 0
		}

		// Risk distribution
		riskRows, _ := pool.Query(ctx, `
			SELECT max_severity, COUNT(*) as count
			FROM risk_scores
			GROUP BY max_severity
		`)
		if riskRows != nil {
			defer riskRows.Close()
			for riskRows.Next() {
				var severity string
				var count int
				if riskRows.Scan(&severity, &count) == nil {
					stats.RiskDistribution[severity] = count
				}
			}
		}

		// Recent syncs summary
		syncRows, _ := pool.Query(ctx, `
			SELECT DISTINCT ON (source_system, connector_name)
				source_system,
				connector_name,
				finished_at,
				status,
				(SELECT COUNT(*) FROM identity_sources WHERE source_system = sync_runs.source_system) as identity_count
			FROM sync_runs
			WHERE finished_at IS NOT NULL
			ORDER BY source_system, connector_name, finished_at DESC
			LIMIT 10
		`)
		if syncRows != nil {
			defer syncRows.Close()
			for syncRows.Next() {
				var sync SyncSummaryDTO
				var finishedAt *string
				if syncRows.Scan(&sync.SourceSystem, &sync.ConnectorName, &finishedAt, &sync.Status, &sync.IdentityCount) == nil {
					if finishedAt != nil {
						sync.LastSyncAt = *finishedAt
					}
					stats.RecentSyncs = append(stats.RecentSyncs, sync)
				}
			}
		}

		// Top risk issues (most common risk flags)
		issueRows, _ := pool.Query(ctx, `
			SELECT 
				rule_key,
				severity,
				is_deadend,
				message,
				COUNT(*) as count
			FROM risk_flags
			WHERE cleared_at IS NULL
			GROUP BY rule_key, severity, is_deadend, message
			ORDER BY 
				CASE severity
					WHEN 'critical' THEN 1
					WHEN 'high' THEN 2
					WHEN 'medium' THEN 3
					ELSE 4
				END,
				count DESC
			LIMIT 10
		`)
		if issueRows != nil {
			defer issueRows.Close()
			for issueRows.Next() {
				var issue RiskIssueSummary
				if issueRows.Scan(&issue.RuleKey, &issue.Severity, &issue.IsDeadend, &issue.Message, &issue.Count) == nil {
					stats.TopRiskIssues = append(stats.TopRiskIssues, issue)
				}
			}
		}

		// Identities without MFA (privileged identities that don't have MFA)
		noMfaRows, _ := pool.Query(ctx, `
			SELECT DISTINCT
				i.id,
				i.email,
				i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score,
				rs.max_severity
			FROM identities i
			JOIN identity_role ir ON ir.identity_id = i.id
			JOIN roles r ON r.id = ir.role_id
			LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(r.name) LIKE '%admin%' 
			   OR LOWER(r.name) LIKE '%privileged%'
			   OR LOWER(r.name) LIKE '%root%'
			   OR LOWER(r.name) LIKE '%super%')
			  AND NOT EXISTS (
				SELECT 1 FROM identity_sources is3
				WHERE is3.identity_id = i.id
				  AND (is3.metadata->>'mfa_enabled' = 'true' OR is3.metadata->>'mfa_configured' = 'true')
			  )
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC NULLS LAST
			LIMIT 50
		`)
		if noMfaRows != nil {
			defer noMfaRows.Close()
			for noMfaRows.Next() {
				var ident IdentitySummary
				var sourceSystems []string
				var riskScore *int
				var maxSeverity *string
				if noMfaRows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity) == nil {
					if sourceSystems == nil {
						sourceSystems = []string{}
					}
					ident.SourceSystems = sourceSystems
					ident.RiskScore = riskScore
					ident.MaxSeverity = maxSeverity
					stats.NoMfaIdentities = append(stats.NoMfaIdentities, ident)
				}
			}
		}

		// Deadend identities (with reason why they are deadends)
		deadendRows, _ := pool.Query(ctx, `
			SELECT
				i.id,
				i.email,
				i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score,
				rs.max_severity,
				(SELECT rf.message FROM risk_flags rf WHERE rf.identity_id = i.id AND rf.is_deadend = true AND rf.cleared_at IS NULL ORDER BY rf.created_at DESC LIMIT 1) as deadend_reason
			FROM identities i
			JOIN risk_flags rf ON rf.identity_id = i.id
			LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE rf.is_deadend = true AND rf.cleared_at IS NULL
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC NULLS LAST
			LIMIT 50
		`)
		if deadendRows != nil {
			defer deadendRows.Close()
			for deadendRows.Next() {
				var ident IdentitySummary
				var sourceSystems []string
				var riskScore *int
				var maxSeverity *string
				var deadendReason *string
				if deadendRows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity, &deadendReason) == nil {
					if sourceSystems == nil {
						sourceSystems = []string{}
					}
					ident.SourceSystems = sourceSystems
					ident.RiskScore = riskScore
					ident.MaxSeverity = maxSeverity
					ident.DeadendReason = deadendReason
					stats.DeadendIdentities = append(stats.DeadendIdentities, ident)
				}
			}
		}

		// Cross-cloud admins list
		crossCloudRows, _ := pool.Query(ctx, `
			SELECT DISTINCT
				i.id,
				i.email,
				i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score,
				rs.max_severity
			FROM identities i
			JOIN identity_sources is2 ON is2.identity_id = i.id
			JOIN identity_role ir ON ir.identity_id = i.id
			JOIN roles r ON r.id = ir.role_id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(r.name) LIKE '%admin%' 
			   OR LOWER(r.name) LIKE '%privileged%'
			   OR LOWER(r.name) LIKE '%root%')
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			HAVING COUNT(DISTINCT is2.source_system) > 1
			ORDER BY rs.score DESC NULLS LAST
			LIMIT 50
		`)
		if crossCloudRows != nil {
			defer crossCloudRows.Close()
			for crossCloudRows.Next() {
				var ident IdentitySummary
				var sourceSystems []string
				var riskScore *int
				var maxSeverity *string
				if crossCloudRows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity) == nil {
					if sourceSystems == nil {
						sourceSystems = []string{}
					}
					ident.SourceSystems = sourceSystems
					ident.RiskScore = riskScore
					ident.MaxSeverity = maxSeverity
					stats.CrossCloudAdminList = append(stats.CrossCloudAdminList, ident)
				}
			}
		}

		// Privileged identities (those with admin/privileged roles via direct assignment OR via groups)
		// Use effective permissions view which includes both direct and group-based access
		privilegedRows, _ := pool.Query(ctx, `
			SELECT DISTINCT
				i.id,
				i.email,
				i.display_name,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems,
				rs.score,
				rs.max_severity
			FROM identities i
			JOIN identity_effective_permissions iep ON iep.identity_id = i.id
			LEFT JOIN identity_sources is2 ON is2.identity_id = i.id
			LEFT JOIN risk_scores rs ON rs.identity_id = i.id
			WHERE (LOWER(iep.role_name) LIKE '%admin%' 
			   OR LOWER(iep.role_name) LIKE '%privileged%'
			   OR LOWER(iep.role_name) LIKE '%root%'
			   OR LOWER(iep.role_name) LIKE '%super%'
			   OR iep.privilege_level IN ('admin', 'administrator', 'super_admin'))
			GROUP BY i.id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC NULLS LAST
			LIMIT 50
		`)
		if privilegedRows != nil {
			defer privilegedRows.Close()
			for privilegedRows.Next() {
				var ident IdentitySummary
				var sourceSystems []string
				var riskScore *int
				var maxSeverity *string
				if privilegedRows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &sourceSystems, &riskScore, &maxSeverity) == nil {
					if sourceSystems == nil {
						sourceSystems = []string{}
					}
					ident.SourceSystems = sourceSystems
					ident.RiskScore = riskScore
					ident.MaxSeverity = maxSeverity
					stats.PrivilegedIdentities = append(stats.PrivilegedIdentities, ident)
				}
			}
		}

		// Return the stats (even if some queries failed, return what we have)
		c.JSON(http.StatusOK, stats)
	}
}
