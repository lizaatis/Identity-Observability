package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthResponse for GET /health
type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database,omitempty"`
	Neo4j    string `json:"neo4j,omitempty"`
}

// IdentitiesListResponse for GET /api/v1/identities
type IdentitiesListResponse struct {
	Identities []IdentityListItemDTO `json:"identities"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
}

type IdentityListItemDTO struct {
	ID          int64   `json:"id"`
	EmployeeID  *string `json:"employee_id,omitempty"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	Status      string  `json:"status"`
}

// EffectivePermissionsResponse for GET /api/v1/identities/:id/effective-permissions
type EffectivePermissionsResponse struct {
	IdentityID           int64                    `json:"identity_id"`
	EffectivePermissions []EffectivePermissionDTO `json:"effective_permissions"`
}

// healthCheck returns process liveness plus Postgres and Neo4j connectivity for operators.
func healthCheck(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unavailable: " + err.Error()
		}
		neoStatus := "not_initialized"
		if graphClient != nil {
			if err := graphClient.Ping(ctx); err != nil {
				neoStatus = "unavailable: " + err.Error()
			} else {
				neoStatus = "ok"
			}
		}
		status := "healthy"
		if dbStatus != "ok" {
			status = "degraded"
		}
		c.JSON(http.StatusOK, HealthResponse{
			Status:   status,
			Version:  "1.0.0",
			Database: dbStatus,
			Neo4j:    neoStatus,
		})
	}
}

// List identities with pagination
func listIdentities(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Parse pagination params
		page := 1
		pageSize := 50
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}
		if ps := c.Query("page_size"); ps != "" {
			if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
				pageSize = parsed
			}
		}

		// Optional filters
		sourceSystem := c.Query("source_system")
		status := c.Query("status")

		offset := (page - 1) * pageSize

		// Build query
		query := `
			SELECT id, employee_id, email, display_name, status
			FROM identities
			WHERE 1=1`
		args := []interface{}{}
		argPos := 1

		if sourceSystem != "" {
			query += ` AND EXISTS (
				SELECT 1 FROM identity_sources 
				WHERE identity_sources.identity_id = identities.id 
				AND identity_sources.source_system = $` + strconv.Itoa(argPos) + `
			)`
			args = append(args, sourceSystem)
			argPos++
		}

		if status != "" {
			query += ` AND status = $` + strconv.Itoa(argPos)
			args = append(args, status)
			argPos++
		}

		query += ` ORDER BY id LIMIT $` + strconv.Itoa(argPos) + ` OFFSET $` + strconv.Itoa(argPos+1)
		args = append(args, pageSize, offset)

		rows, err := pool.Query(ctx, query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var identities []IdentityListItemDTO
		for rows.Next() {
			var ident IdentityListItemDTO
			if err := rows.Scan(&ident.ID, &ident.EmployeeID, &ident.Email, &ident.DisplayName, &ident.Status); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			identities = append(identities, ident)
		}

		// Get total count
		countQuery := `SELECT COUNT(*) FROM identities WHERE 1=1`
		countArgs := []interface{}{}
		countArgPos := 1

		if sourceSystem != "" {
			countQuery += ` AND EXISTS (
				SELECT 1 FROM identity_sources 
				WHERE identity_sources.identity_id = identities.id 
				AND identity_sources.source_system = $` + strconv.Itoa(countArgPos) + `
			)`
			countArgs = append(countArgs, sourceSystem)
			countArgPos++
		}

		if status != "" {
			countQuery += ` AND status = $` + strconv.Itoa(countArgPos)
			countArgs = append(countArgs, status)
		}

		var total int
		if err := pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, IdentitiesListResponse{
			Identities: identities,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
		})
	}
}

// Get effective permissions for an identity
func getEffectivePermissions(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}
		ctx := c.Request.Context()

		// Verify identity exists
		var exists bool
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identities WHERE id = $1)`, id).Scan(&exists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "identity not found"})
			return
		}

		// Get effective permissions
		rows, err := pool.Query(ctx, `
			SELECT permission_id, permission_name, resource_type, permission_source_system,
			       path_type, role_id, role_name, privilege_level, role_source_system, group_id, group_name
			FROM identity_effective_permissions WHERE identity_id = $1`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var perms []EffectivePermissionDTO
		for rows.Next() {
			var p EffectivePermissionDTO
			if err := rows.Scan(&p.PermissionID, &p.PermissionName, &p.ResourceType, &p.PermissionSource,
				&p.PathType, &p.RoleID, &p.RoleName, &p.PrivilegeLevel, &p.RoleSourceSystem, &p.GroupID, &p.GroupName); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			perms = append(perms, p)
		}

		c.JSON(http.StatusOK, EffectivePermissionsResponse{
			IdentityID:           id,
			EffectivePermissions: perms,
		})
	}
}
