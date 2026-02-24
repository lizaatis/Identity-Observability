package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dataPath := os.Getenv("MOCK_DATA_PATH")
	if dataPath == "" {
		dataPath = "data/test_dataset.json"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	data, err := os.ReadFile(dataPath)
	if err != nil {
		log.Fatalf("read dataset: %v", err)
	}
	var ds MockDataset
	if err := json.Unmarshal(data, &ds); err != nil {
		log.Fatalf("parse dataset: %v", err)
	}

	now := time.Now().UTC()
	identityByKey := make(map[string]int64) // "employee_id:E001" or "email:alice@example.com" -> identity id

	// 1. Resolve and upsert identities + identity_sources
	for _, u := range ds.Users {
		var identityID int64
		// Resolve: by employee_id first, then email (in-memory from this run, then DB)
		if u.EmployeeID != "" {
			identityID = identityByKey["emp:"+u.EmployeeID]
		}
		if identityID == 0 {
			identityID = identityByKey["email:"+u.Email]
		}
		if identityID == 0 {
			// Resolve from DB (for idempotent re-runs)
			if u.EmployeeID != "" {
				_ = pool.QueryRow(ctx, `SELECT id FROM identities WHERE employee_id = $1`, u.EmployeeID).Scan(&identityID)
			}
			if identityID == 0 {
				_ = pool.QueryRow(ctx, `SELECT id FROM identities WHERE email = $1`, u.Email).Scan(&identityID)
			}
		}
		if identityID == 0 {
			// Create new canonical identity
			err := pool.QueryRow(ctx, `
				INSERT INTO identities (employee_id, email, display_name, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $5)
				RETURNING id`,
				nullIfEmpty(u.EmployeeID), u.Email, nullIfEmpty(u.DisplayName), u.Status, now,
			).Scan(&identityID)
			if err != nil {
				log.Fatalf("insert identity %s: %v", u.Email, err)
			}
			if u.EmployeeID != "" {
				identityByKey["emp:"+u.EmployeeID] = identityID
			}
			identityByKey["email:"+u.Email] = identityID
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO identity_sources (identity_id, source_system, source_user_id, source_status, confidence, synced_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 1.0, $5, $5, $5)
			ON CONFLICT (source_system, source_user_id) DO UPDATE SET
				identity_id = $1, source_status = $4, synced_at = $5, updated_at = $5`,
			identityID, u.SourceSystem, u.SourceID, u.Status, now,
		)
		if err != nil {
			log.Fatalf("upsert identity_sources %s %s: %v", u.SourceSystem, u.SourceID, err)
		}
	}
	log.Println("identities and identity_sources upserted")

	// 2. Upsert groups, roles, permissions and build source -> id maps
	groupID := make(map[string]int64)   // key: source_system|source_id
	roleID := make(map[string]int64)
	permID := make(map[string]int64)

	for _, g := range ds.Groups {
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO groups (name, source_system, source_id, synced_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4, $4)
			ON CONFLICT (source_system, source_id) DO UPDATE SET name = $1, synced_at = $4, updated_at = $4
			RETURNING id`, g.Name, g.SourceSystem, g.SourceID, now,
		).Scan(&id)
		if err != nil {
			log.Fatalf("upsert group %s %s: %v", g.SourceSystem, g.SourceID, err)
		}
		groupID[g.SourceSystem+"|"+g.SourceID] = id
	}
	for _, r := range ds.Roles {
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO roles (name, privilege_level, source_system, source_id, synced_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5, $5)
			ON CONFLICT (source_system, source_id) DO UPDATE SET name = $1, privilege_level = $2, synced_at = $5, updated_at = $5
			RETURNING id`, r.Name, r.PrivilegeLevel, r.SourceSystem, r.SourceID, now,
		).Scan(&id)
		if err != nil {
			log.Fatalf("upsert role %s %s: %v", r.SourceSystem, r.SourceID, err)
		}
		roleID[r.SourceSystem+"|"+r.SourceID] = id
	}
	for _, p := range ds.Permissions {
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO permissions (name, resource_type, source_system, source_id, synced_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5, $5)
			ON CONFLICT (source_system, source_id) DO UPDATE SET name = $1, resource_type = $2, synced_at = $5, updated_at = $5
			RETURNING id`, p.Name, nullIfEmpty(p.ResourceType), p.SourceSystem, p.SourceID, now,
		).Scan(&id)
		if err != nil {
			log.Fatalf("upsert permission %s %s: %v", p.SourceSystem, p.SourceID, err)
		}
		permID[p.SourceSystem+"|"+p.SourceID] = id
	}
	log.Println("groups, roles, permissions upserted")

	// Resolve identity_id from (source_system, source_user_id)
	getIdentityID := func(system, userID string) int64 {
		var id int64
		err := pool.QueryRow(ctx, `SELECT identity_id FROM identity_sources WHERE source_system = $1 AND source_user_id = $2`, system, userID).Scan(&id)
		if err != nil {
			log.Fatalf("resolve identity %s %s: %v", system, userID, err)
		}
		return id
	}

	// 3. Upsert relationship tables
	for _, ig := range ds.IdentityGroup {
		identityID := getIdentityID(ig.SourceSystem, ig.SourceUserID)
		gid := groupID[ig.SourceSystem+"|"+ig.SourceGroupID]
		_, err := pool.Exec(ctx, `
			INSERT INTO identity_group (identity_id, group_id, source_system, source_id, synced_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (identity_id, group_id, source_system) DO UPDATE SET source_id = $4, synced_at = $5`,
			identityID, gid, ig.SourceSystem, ig.SourceUserID+"|"+ig.SourceGroupID, now,
		)
		if err != nil {
			log.Fatalf("upsert identity_group: %v", err)
		}
	}
	for _, ir := range ds.IdentityRole {
		identityID := getIdentityID(ir.SourceSystem, ir.SourceUserID)
		rid := roleID[ir.SourceSystem+"|"+ir.SourceRoleID]
		_, err := pool.Exec(ctx, `
			INSERT INTO identity_role (identity_id, role_id, source_system, source_id, synced_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (identity_id, role_id, source_system) DO UPDATE SET source_id = $4, synced_at = $5`,
			identityID, rid, ir.SourceSystem, ir.SourceUserID+"|"+ir.SourceRoleID, now,
		)
		if err != nil {
			log.Fatalf("upsert identity_role: %v", err)
		}
	}
	for _, gr := range ds.GroupRole {
		gid := groupID[gr.SourceSystem+"|"+gr.SourceGroupID]
		rid := roleID[gr.SourceSystem+"|"+gr.SourceRoleID]
		_, err := pool.Exec(ctx, `
			INSERT INTO group_role (group_id, role_id, source_system, source_id, synced_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (group_id, role_id, source_system) DO UPDATE SET source_id = $4, synced_at = $5`,
			gid, rid, gr.SourceSystem, gr.SourceGroupID+"|"+gr.SourceRoleID, now,
		)
		if err != nil {
			log.Fatalf("upsert group_role: %v", err)
		}
	}
	for _, rp := range ds.RolePermission {
		rid := roleID[rp.SourceSystem+"|"+rp.SourceRoleID]
		pid := permID[rp.SourceSystem+"|"+rp.SourcePermissionID]
		_, err := pool.Exec(ctx, `
			INSERT INTO role_permission (role_id, permission_id, source_system, source_id, synced_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (role_id, permission_id, source_system) DO UPDATE SET source_id = $4, synced_at = $5`,
			rid, pid, rp.SourceSystem, rp.SourceRoleID+"|"+rp.SourcePermissionID, now,
		)
		if err != nil {
			log.Fatalf("upsert role_permission: %v", err)
		}
	}
	log.Println("relationship tables upserted")
	fmt.Println("Mock connector run complete.")
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
