package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdentityResponse is the JSON shape for GET /api/v1/identities/:id
type IdentityResponse struct {
	Identity             IdentityDTO             `json:"identity"`
	Sources              []IdentitySourceDTO     `json:"sources"`
	EffectivePermissions []EffectivePermissionDTO `json:"effective_permissions"`
	Lineage              []PermissionLineageDTO  `json:"lineage"`
	SystemSummary        []SystemSummaryItem     `json:"system_summary"`
	DeadendSummary       DeadendSummary          `json:"deadend_summary"`
	StitchingSummary     StitchingSummary        `json:"stitching_summary"`
}

// SystemSummaryItem is one line in the identity dossier strip (e.g. "Okta: Super Admin (MFA: Yes)")
type SystemSummaryItem struct {
	System         string   `json:"system"`
	IsAdmin        bool     `json:"is_admin"`
	AdminRoles     []string `json:"admin_roles"`
	MfaEnabled     *bool    `json:"mfa_enabled,omitempty"`
	LastSyncAt     string   `json:"last_sync_at"`
	DataFreshness  string   `json:"data_freshness"` // fresh, stale, very_stale, unknown
}

// StitchingSummary explains why this identity is treated as one person across systems.
type StitchingSummary struct {
	Confidence  string   `json:"confidence"` // high, medium, low, single_source
	Reasons     []string `json:"reasons"`
	NeedsReview bool     `json:"needs_review"`
	SourceCount int      `json:"source_count"`
}

// DeadendSummary for the dossier strip (e.g. "Deadends: 1 (orphaned user)")
type DeadendSummary struct {
	Count   int      `json:"count"`
	Reasons []string `json:"reasons"`
}

type IdentityDTO struct {
	ID          int64   `json:"id"`
	EmployeeID  *string `json:"employee_id,omitempty"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type IdentitySourceDTO struct {
	SourceSystem    string  `json:"source_system"`
	SourceUserID    string  `json:"source_user_id"`
	SourceStatus    string  `json:"source_status"`
	SyncedAt        string  `json:"synced_at"`
	MfaEnabled      *string `json:"mfa_enabled,omitempty"` // "true"/"false" from metadata
	DataFreshness   string  `json:"data_freshness"`         // fresh, stale, very_stale, unknown
}

type EffectivePermissionDTO struct {
	PermissionID       int64   `json:"permission_id"`
	PermissionName     string  `json:"permission_name"`
	ResourceType      *string `json:"resource_type,omitempty"`
	PermissionSource  string  `json:"permission_source_system"`
	PathType          string  `json:"path_type"`
	RoleID            int64   `json:"role_id"`
	RoleName          string  `json:"role_name"`
	PrivilegeLevel    string  `json:"privilege_level"`
	RoleSourceSystem  string  `json:"role_source_system"`
	GroupID           *int64  `json:"group_id,omitempty"`
	GroupName         *string `json:"group_name,omitempty"`
}

type PermissionLineageDTO struct {
	PermissionID   int64    `json:"permission_id"`
	PermissionName string  `json:"permission_name"`
	Path          []HopDTO `json:"path"`
}

type HopDTO struct {
	HopType   string  `json:"hop_type"`
	HopName   string  `json:"hop_name"`
	HopDetail *string `json:"hop_detail,omitempty"`
	Ord       int     `json:"ord"`
}

func identityByID(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}
		ctx := c.Request.Context()

		// 1. Identity row
		var ident IdentityDTO
		err = pool.QueryRow(ctx, `
			SELECT id, employee_id, email, display_name, status, created_at::text, updated_at::text
			FROM identities WHERE id = $1`, id,
		).Scan(&ident.ID, &ident.EmployeeID, &ident.Email, &ident.DisplayName, &ident.Status, &ident.CreatedAt, &ident.UpdatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "identity not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// 2. Identity sources (include MFA from metadata if column exists)
		rows, err := pool.Query(ctx, `
			SELECT source_system, source_user_id, source_status, synced_at::text,
			       COALESCE(metadata->>'mfa_enabled', metadata->>'MFA_ENABLED') AS mfa_enabled
			FROM identity_sources WHERE identity_id = $1`, id)
		if err != nil {
			// Fallback when metadata column does not exist (run migration 008)
			rows, err = pool.Query(ctx, `
				SELECT source_system, source_user_id, source_status, synced_at::text, NULL::text
				FROM identity_sources WHERE identity_id = $1`, id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		var sources []IdentitySourceDTO
		for rows.Next() {
			var s IdentitySourceDTO
			if err := rows.Scan(&s.SourceSystem, &s.SourceUserID, &s.SourceStatus, &s.SyncedAt, &s.MfaEnabled); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			s.DataFreshness = dataFreshnessFromSyncedAt(s.SyncedAt)
			sources = append(sources, s)
		}
		rows.Close()

		// 3. Effective permissions (from view)
		rows, err = pool.Query(ctx, `
			SELECT permission_id, permission_name, resource_type, permission_source_system,
			       path_type, role_id, role_name, privilege_level, role_source_system, group_id, group_name
			FROM identity_effective_permissions WHERE identity_id = $1`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var perms []EffectivePermissionDTO
		for rows.Next() {
			var p EffectivePermissionDTO
			if err := rows.Scan(&p.PermissionID, &p.PermissionName, &p.ResourceType, &p.PermissionSource,
				&p.PathType, &p.RoleID, &p.RoleName, &p.PrivilegeLevel, &p.RoleSourceSystem, &p.GroupID, &p.GroupName); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			perms = append(perms, p)
		}
		rows.Close()

		// 4. Lineage: identity_access_lineage grouped by permission_id, ordered by ord
		rows, err = pool.Query(ctx, `
			SELECT permission_id, hop_type, hop_name, hop_detail, ord
			FROM identity_access_lineage WHERE identity_id = $1 ORDER BY permission_id, ord`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		type lineageRow struct {
			PermissionID int64
			HopType     string
			HopName     string
			HopDetail   *string
			Ord         int
		}
		var lineageRows []lineageRow
		for rows.Next() {
			var r lineageRow
			if err := rows.Scan(&r.PermissionID, &r.HopType, &r.HopName, &r.HopDetail, &r.Ord); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			lineageRows = append(lineageRows, r)
		}
		rows.Close()

		// Build lineage per permission (path = ordered hops); dedupe by permission_id path
		pathByPerm := make(map[int64][]HopDTO)
		permNames := make(map[int64]string)
		for _, p := range perms {
			permNames[p.PermissionID] = p.PermissionName
		}
		for _, r := range lineageRows {
			pathByPerm[r.PermissionID] = append(pathByPerm[r.PermissionID], HopDTO{
				HopType: r.HopType, HopName: r.HopName, HopDetail: r.HopDetail, Ord: r.Ord,
			})
		}
		var lineage []PermissionLineageDTO
		for permID, path := range pathByPerm {
			lineage = append(lineage, PermissionLineageDTO{
				PermissionID:   permID,
				PermissionName: permNames[permID],
				Path:           path,
			})
		}

		// 5. Admin roles per system (from effective permissions: privilege_level=admin or role name contains admin/super/owner)
		adminRolesBySystem := make(map[string]map[string]struct{})
		for _, p := range perms {
			sys := p.RoleSourceSystem
			if sys == "" {
				continue
			}
			lower := strings.ToLower(p.RoleName)
			isPriv := p.PrivilegeLevel == "admin" || strings.Contains(lower, "admin") ||
				strings.Contains(lower, "super") || strings.Contains(lower, "owner")
			if !isPriv {
				continue
			}
			if adminRolesBySystem[sys] == nil {
				adminRolesBySystem[sys] = make(map[string]struct{})
			}
			adminRolesBySystem[sys][p.RoleName] = struct{}{}
		}

		// 6. System summary for dossier strip
		systemSummary := make([]SystemSummaryItem, 0, len(sources))
		for _, s := range sources {
			roles := make([]string, 0)
			if m := adminRolesBySystem[s.SourceSystem]; m != nil {
				for r := range m {
					roles = append(roles, r)
				}
			}
			var mfa *bool
			if s.MfaEnabled != nil {
				b := strings.ToLower(*s.MfaEnabled) == "true" || *s.MfaEnabled == "1"
				mfa = &b
			}
			systemSummary = append(systemSummary, SystemSummaryItem{
				System:        s.SourceSystem,
				IsAdmin:       len(roles) > 0,
				AdminRoles:    roles,
				MfaEnabled:    mfa,
				LastSyncAt:    s.SyncedAt,
				DataFreshness: dataFreshnessFromSyncedAt(s.SyncedAt),
			})
		}

		stitch := buildStitchingSummary(ident, sources)
		var needsReview bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM stitching_review_queue
				WHERE identity_id = $1 AND status = 'pending'
			)`, id).Scan(&needsReview); err == nil {
			stitch.NeedsReview = needsReview
		}

		// 7. Deadend summary (risk_flags where is_deadend, not cleared)
		var deadendSummary DeadendSummary
		deadRows, err := pool.Query(ctx, `
			SELECT rule_key, message FROM risk_flags
			WHERE identity_id = $1 AND is_deadend = true AND (cleared_at IS NULL OR cleared_at > NOW())`, id)
		if err == nil {
			for deadRows.Next() {
				var ruleKey, msg string
				if err := deadRows.Scan(&ruleKey, &msg); err != nil {
					break
				}
				deadendSummary.Count++
				if msg != "" {
					deadendSummary.Reasons = append(deadendSummary.Reasons, msg)
				} else {
					deadendSummary.Reasons = append(deadendSummary.Reasons, ruleKey)
				}
			}
			deadRows.Close()
		}

		c.JSON(http.StatusOK, IdentityResponse{
			Identity:             ident,
			Sources:              sources,
			EffectivePermissions: perms,
			Lineage:              lineage,
			SystemSummary:        systemSummary,
			DeadendSummary:       deadendSummary,
			StitchingSummary:     stitch,
		})
	}
}

func dataFreshnessFromSyncedAt(syncedAt string) string {
	if syncedAt == "" {
		return "unknown"
	}
	t, err := time.Parse(time.RFC3339, syncedAt)
	if err != nil {
		return "unknown"
	}
	age := time.Since(t)
	if age < 24*time.Hour {
		return "fresh"
	}
	if age < 7*24*time.Hour {
		return "stale"
	}
	return "very_stale"
}

func buildStitchingSummary(ident IdentityDTO, sources []IdentitySourceDTO) StitchingSummary {
	s := StitchingSummary{
		SourceCount: len(sources),
		Reasons:     []string{},
	}
	if len(sources) <= 1 {
		s.Confidence = "single_source"
		s.Reasons = append(s.Reasons, "Only one linked source record in this environment — add more connectors to see cross-system stitching.")
		return s
	}
	s.Reasons = append(s.Reasons, "Multiple source systems resolve to this single canonical identity row (same person in Postgres).")
	if ident.EmployeeID != nil && strings.TrimSpace(*ident.EmployeeID) != "" {
		s.Confidence = "high"
		s.Reasons = append(s.Reasons, "Employee ID is present on the canonical record — strong key when connectors provide it.")
	} else {
		s.Confidence = "medium"
		s.Reasons = append(s.Reasons, "Typical correlation uses email and per-connector IDs stored in identity_sources.")
	}
	if ident.Email != "" {
		s.Reasons = append(s.Reasons, "Canonical email: "+ident.Email+" — used as the primary human-readable join key.")
	}
	return s
}
