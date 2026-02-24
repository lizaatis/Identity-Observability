package main

import (
	"errors"
	"net/http"
	"strconv"

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
	SourceSystem string `json:"source_system"`
	SourceUserID string `json:"source_user_id"`
	SourceStatus string `json:"source_status"`
	SyncedAt     string `json:"synced_at"`
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

		// 2. Identity sources
		rows, err := pool.Query(ctx, `
			SELECT source_system, source_user_id, source_status, synced_at::text
			FROM identity_sources WHERE identity_id = $1`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var sources []IdentitySourceDTO
		for rows.Next() {
			var s IdentitySourceDTO
			if err := rows.Scan(&s.SourceSystem, &s.SourceUserID, &s.SourceStatus, &s.SyncedAt); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
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

		c.JSON(http.StatusOK, IdentityResponse{
			Identity:             ident,
			Sources:              sources,
			EffectivePermissions: perms,
			Lineage:              lineage,
		})
	}
}
