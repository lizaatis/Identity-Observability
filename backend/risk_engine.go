package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RiskRule defines a risk scoring rule
type RiskRule struct {
	Key      string
	Weight   int
	Severity string // "low", "medium", "high", "critical"
	IsDeadend bool
}

// RiskScore represents a computed risk score
type RiskScore struct {
	IdentityID  int64
	Score       int
	MaxSeverity string
	Flags       []RiskFlag
	ComputedAt  time.Time
}

// RiskFlag represents a single risk flag
type RiskFlag struct {
	RuleKey   string
	Severity  string
	IsDeadend bool
	Message   string
	Metadata  map[string]interface{}
}

// RiskEngine computes risk scores for identities
type RiskEngine struct {
	pool        *pgxpool.Pool
	mainIdPKey  string // e.g. "okta_primary"
	rules       []RiskRule
}

// NewRiskEngine creates a new risk engine
func NewRiskEngine(pool *pgxpool.Pool, mainIdPKey string) *RiskEngine {
	return &RiskEngine{
		pool:       pool,
		mainIdPKey: mainIdPKey,
		rules: []RiskRule{
			{Key: "cross_system_admin", Weight: 40, Severity: "high", IsDeadend: false},
			{Key: "disabled_drift", Weight: 50, Severity: "critical", IsDeadend: false},
			{Key: "privileged_without_mfa", Weight: 30, Severity: "high", IsDeadend: false},
			{Key: "deadend_orphaned_user", Weight: 50, Severity: "critical", IsDeadend: true},
			{Key: "deadend_orphaned_group", Weight: 40, Severity: "high", IsDeadend: true},
			{Key: "deadend_stale_role", Weight: 20, Severity: "medium", IsDeadend: true},
			{Key: "deadend_disconnected_permission", Weight: 30, Severity: "medium", IsDeadend: true},
		},
	}
}

// ComputeRiskScore computes risk score for a single identity
func (re *RiskEngine) ComputeRiskScore(ctx context.Context, identityID int64) (*RiskScore, error) {
	flags := []RiskFlag{}

	// Rule 1: Cross-System Admin
	if flag := re.checkCrossSystemAdmin(ctx, identityID); flag != nil {
		flags = append(flags, *flag)
	}

	// Rule 2: Disabled Drift
	if flag := re.checkDisabledDrift(ctx, identityID); flag != nil {
		flags = append(flags, *flag)
	}

	// Rule 3: Privileged Without MFA
	if flag := re.checkPrivilegedWithoutMFA(ctx, identityID); flag != nil {
		flags = append(flags, *flag)
	}

	// Rule 4: Deadend - Orphaned User
	if flag := re.checkOrphanedUser(ctx, identityID); flag != nil {
		flags = append(flags, *flag)
	}

	// Rule 5: Deadend - Orphaned Group
	if flags := re.checkOrphanedGroups(ctx, identityID); len(flags) > 0 {
		flags = append(flags, flags...)
	}

	// Rule 6: Deadend - Stale Roles
	if flags := re.checkStaleRoles(ctx, identityID); len(flags) > 0 {
		flags = append(flags, flags...)
	}

	// Rule 7: Deadend - Disconnected Permissions
	if flags := re.checkDisconnectedPermissions(ctx, identityID); len(flags) > 0 {
		flags = append(flags, flags...)
	}

	// Calculate score
	score := 0
	maxSeverity := "low"
	for _, flag := range flags {
		// Find rule weight
		for _, rule := range re.rules {
			if rule.Key == flag.RuleKey {
				score += rule.Weight
				break
			}
		}
		// Track max severity
		if severityValue(flag.Severity) > severityValue(maxSeverity) {
			maxSeverity = flag.Severity
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return &RiskScore{
		IdentityID:  identityID,
		Score:       score,
		MaxSeverity: maxSeverity,
		Flags:       flags,
		ComputedAt:  time.Now(),
	}, nil
}

// checkCrossSystemAdmin: Admin role in 2+ systems
func (re *RiskEngine) checkCrossSystemAdmin(ctx context.Context, identityID int64) *RiskFlag {
	var count int
	err := re.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT r.source_system)
		FROM identity_role ir
		JOIN roles r ON r.id = ir.role_id
		WHERE ir.identity_id = $1
		  AND r.privilege_level IN ('admin', 'administrator', 'super_admin')
	`, identityID).Scan(&count)

	if err != nil || count < 2 {
		return nil
	}

	return &RiskFlag{
		RuleKey:   "cross_system_admin",
		Severity:  "high",
		IsDeadend: false,
		Message:   fmt.Sprintf("Admin role in %d different systems", count),
		Metadata:  map[string]interface{}{"system_count": count},
	}
}

// checkDisabledDrift: Disabled in main IdP but active elsewhere
func (re *RiskEngine) checkDisabledDrift(ctx context.Context, identityID int64) *RiskFlag {
	var mainDisabled bool
	var otherActive bool

	// Check main IdP status
	err := re.pool.QueryRow(ctx, `
		SELECT source_status = 'disabled'
		FROM identity_sources
		WHERE identity_id = $1 AND source_system = $2
	`, identityID, re.mainIdPKey).Scan(&mainDisabled)

	if err == pgx.ErrNoRows || !mainDisabled {
		return nil
	}

	// Check if active in other systems
	err = re.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM identity_sources
			WHERE identity_id = $1
			  AND source_system != $2
			  AND source_status = 'active'
		)
	`, identityID, re.mainIdPKey).Scan(&otherActive)

	if err != nil || !otherActive {
		return nil
	}

	return &RiskFlag{
		RuleKey:   "disabled_drift",
		Severity:  "critical",
		IsDeadend: false,
		Message:   "Disabled in main IdP but active in other systems",
		Metadata:  map[string]interface{}{"main_idp": re.mainIdPKey},
	}
}

// checkPrivilegedWithoutMFA: Admin/privileged role without MFA
func (re *RiskEngine) checkPrivilegedWithoutMFA(ctx context.Context, identityID int64) *RiskFlag {
	// Check if identity has admin/privileged roles
	var hasPrivilegedRole bool
	err := re.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM identity_role ir
			JOIN roles r ON r.id = ir.role_id
			WHERE ir.identity_id = $1
			  AND r.privilege_level IN ('admin', 'administrator', 'super_admin')
		)
	`, identityID).Scan(&hasPrivilegedRole)

	if err != nil || !hasPrivilegedRole {
		return nil
	}

	// Check if MFA is enabled (this would come from identity_sources or a separate MFA table)
	// For now, we'll check if there's an MFA indicator in identity_sources metadata
	// In production, you'd have a dedicated MFA status table or field
	var hasMFA bool
	err = re.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM identity_sources
			WHERE identity_id = $1
			  AND source_status = 'active'
			  AND (metadata->>'mfa_enabled' = 'true' OR metadata->>'mfa_configured' = 'true')
		)
	`, identityID).Scan(&hasMFA)

	// If no MFA metadata found, we can't determine MFA status
	// In a real implementation, you'd sync MFA status from Okta/IdP
	// For now, we'll skip this check if MFA data isn't available
	if err != nil {
		return nil // Can't determine MFA status
	}

	if !hasMFA {
		return &RiskFlag{
			RuleKey:   "privileged_without_mfa",
			Severity:  "high",
			IsDeadend: false,
			Message:   "Has privileged/admin role but MFA not enabled",
			Metadata:  map[string]interface{}{},
		}
	}

	return nil
}

// checkOrphanedUser: Disabled in main IdP but active elsewhere (deadend)
func (re *RiskEngine) checkOrphanedUser(ctx context.Context, identityID int64) *RiskFlag {
	var exists bool
	err := re.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM deadend_orphaned_users WHERE identity_id = $1
		)
	`, identityID).Scan(&exists)

	if err != nil || !exists {
		return nil
	}

	return &RiskFlag{
		RuleKey:   "deadend_orphaned_user",
		Severity:  "critical",
		IsDeadend: true,
		Message:   "Orphaned user: disabled in main IdP but active elsewhere",
		Metadata:  map[string]interface{}{},
	}
}

// checkOrphanedGroups: Groups with no owner but assigned to identity
func (re *RiskEngine) checkOrphanedGroups(ctx context.Context, identityID int64) []RiskFlag {
	rows, err := re.pool.Query(ctx, `
		SELECT group_id, group_name
		FROM deadend_orphaned_group_identities
		WHERE identity_id = $1
	`, identityID)

	if err != nil {
		return nil
	}
	defer rows.Close()

	var flags []RiskFlag
	for rows.Next() {
		var groupID int64
		var groupName string
		if err := rows.Scan(&groupID, &groupName); err != nil {
			continue
		}
		flags = append(flags, RiskFlag{
			RuleKey:   "deadend_orphaned_group",
			Severity:  "high",
			IsDeadend: true,
			Message:   fmt.Sprintf("Member of orphaned group: %s (no owner)", groupName),
			Metadata:  map[string]interface{}{"group_id": groupID, "group_name": groupName},
		})
	}

	return flags
}

// checkStaleRoles: Roles not updated in 90+ days
func (re *RiskEngine) checkStaleRoles(ctx context.Context, identityID int64) []RiskFlag {
	rows, err := re.pool.Query(ctx, `
		SELECT role_id, role_name
		FROM deadend_stale_role_identities
		WHERE identity_id = $1
	`, identityID)

	if err != nil {
		return nil
	}
	defer rows.Close()

	var flags []RiskFlag
	for rows.Next() {
		var roleID int64
		var roleName string
		if err := rows.Scan(&roleID, &roleName); err != nil {
			continue
		}
		flags = append(flags, RiskFlag{
			RuleKey:   "deadend_stale_role",
			Severity:  "medium",
			IsDeadend: true,
			Message:   fmt.Sprintf("Assigned to stale role: %s (>90 days no updates)", roleName),
			Metadata:  map[string]interface{}{"role_id": roleID, "role_name": roleName},
		})
	}

	return flags
}

// checkDisconnectedPermissions: Permissions unreachable from active identities
func (re *RiskEngine) checkDisconnectedPermissions(ctx context.Context, identityID int64) []RiskFlag {
	// This is a bit different - we check if identity has permissions that are disconnected
	rows, err := re.pool.Query(ctx, `
		SELECT DISTINCT p.id, p.name
		FROM identity_effective_permissions iep
		JOIN permissions p ON p.id = iep.permission_id
		JOIN deadend_disconnected_permissions ddp ON ddp.id = p.id
		WHERE iep.identity_id = $1
	`, identityID)

	if err != nil {
		return nil
	}
	defer rows.Close()

	var flags []RiskFlag
	for rows.Next() {
		var permID int64
		var permName string
		if err := rows.Scan(&permID, &permName); err != nil {
			continue
		}
		flags = append(flags, RiskFlag{
			RuleKey:   "deadend_disconnected_permission",
			Severity:  "medium",
			IsDeadend: true,
			Message:   fmt.Sprintf("Has disconnected permission: %s (unreachable from any active identity)", permName),
			Metadata:  map[string]interface{}{"permission_id": permID, "permission_name": permName},
		})
	}

	return flags
}

// StoreRiskScore stores the computed risk score in the database
func (re *RiskEngine) StoreRiskScore(ctx context.Context, score *RiskScore) error {
	// Convert flags to JSON for detail field
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"flags": score.Flags,
	})

	// Store in risk_scores (upsert)
	_, err := re.pool.Exec(ctx, `
		INSERT INTO risk_scores (identity_id, score, max_severity, computed_at, detail)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (identity_id) DO UPDATE SET
			score = EXCLUDED.score,
			max_severity = EXCLUDED.max_severity,
			computed_at = EXCLUDED.computed_at,
			detail = EXCLUDED.detail
	`, score.IdentityID, score.Score, score.MaxSeverity, score.ComputedAt, detailJSON)

	if err != nil {
		return err
	}

	// Store in risk_score_history
	_, err = re.pool.Exec(ctx, `
		INSERT INTO risk_score_history (identity_id, score, max_severity, computed_at, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, score.IdentityID, score.Score, score.MaxSeverity, score.ComputedAt, detailJSON)

	if err != nil {
		return err
	}

	// Store/update risk_flags
	// First, clear old flags for this identity
	_, err = re.pool.Exec(ctx, `
		UPDATE risk_flags SET cleared_at = NOW()
		WHERE identity_id = $1 AND cleared_at IS NULL
	`, score.IdentityID)

	if err != nil {
		return err
	}

	// Insert new flags
	for _, flag := range score.Flags {
		metadataJSON, _ := json.Marshal(flag.Metadata)
		_, err = re.pool.Exec(ctx, `
			INSERT INTO risk_flags (identity_id, rule_key, severity, is_deadend, message, metadata)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, score.IdentityID, flag.RuleKey, flag.Severity, flag.IsDeadend, flag.Message, metadataJSON)
		if err != nil {
			return err
		}
	}

	return nil
}

// Helper function to compare severity levels
func severityValue(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
