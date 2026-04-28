package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

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

	// 1b. Seed MFA metadata on identity_sources (so dashboard MFA coverage shows something)
	_, err = pool.Exec(ctx, `
		UPDATE identity_sources SET metadata = jsonb_build_object('mfa_enabled', 'true', 'mfa_configured', 'true')
		WHERE (source_system = 'okta_mock' AND source_user_id IN ('usr_okta_1','usr_okta_2','usr_okta_3','usr_okta_4','usr_okta_5'))
		   OR (source_system = 'entra_mock' AND source_user_id IN ('usr_entra_1','usr_entra_3','usr_entra_4','usr_entra_5','usr_entra_6'))
		   OR (source_system = 'aws_mock' AND source_user_id IN ('usr_aws_1','usr_aws_2','usr_aws_3'))`)
	if err != nil {
		log.Printf("warning: MFA metadata update skipped (ensure migration 008_identity_sources_metadata.sql is applied): %v", err)
	} else {
		_, _ = pool.Exec(ctx, `
			UPDATE identity_sources SET metadata = jsonb_build_object('mfa_enabled', 'false', 'mfa_configured', 'false')
			WHERE source_system = 'okta_mock' AND source_user_id IN ('usr_okta_6','usr_okta_7','usr_okta_8','usr_okta_9','usr_okta_10')`)
		log.Println("MFA metadata seeded on identity_sources")
	}

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
		err := pool.QueryRow(ctx, `
			SELECT identity_id 
			FROM identity_sources 
			WHERE source_system = $1 AND source_user_id = $2`,
			system, userID,
		).Scan(&id)
		if err != nil {
			// For mock data, log and skip if identity cannot be resolved
			log.Printf("warning: resolve identity %s %s: %v (skipping relationship)", system, userID, err)
			return 0
		}
		return id
	}

	// 3. Upsert relationship tables
	for _, ig := range ds.IdentityGroup {
		identityID := getIdentityID(ig.SourceSystem, ig.SourceUserID)
		if identityID == 0 {
			continue
		}
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
		if identityID == 0 {
			continue
		}
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

	// 4. Seed deadend risk_flags (SailPoint and cross-system) with explanations
	getIdentityIDByEmail := func(email string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM identities WHERE email = $1`, email).Scan(&id); err != nil {
			return 0
		}
		return id
	}
	deadends := []struct {
		email    string
		ruleKey  string
		severity string
		message  string
		meta     string
	}{
		{"bob@example.com", "deadend_orphaned_user", "critical",
			"Disabled in main IdP (Entra) but still has active access in SailPoint and Okta — orphaned user. Remove access in SailPoint/Okta or re-enable in Entra.",
			`{"source_systems":["entra_mock","okta_mock"],"reason":"disabled_in_main_idp_active_elsewhere"}`},
		{"david@example.com", "deadend_orphaned_group", "high",
			"Member of group with no owner in SailPoint — orphaned group deadend. Assign a group owner or remove membership.",
			`{"source_system":"okta_mock","group":"Finance-Managers","reason":"group_has_no_owner"}`},
		{"frank@example.com", "deadend_stale_role", "medium",
			"Has SailPoint role not updated in 90+ days — stale role. Review and update or remove the role assignment.",
			`{"source_system":"aws_mock","reason":"role_not_updated_90_days"}`},
		{"eve@example.com", "deadend_disconnected_permission", "medium",
			"Has permission in SailPoint with no active path from any system — disconnected permission. Revoke or reassign.",
			`{"source_system":"entra_mock","reason":"permission_unreachable_from_active_identity"}`},
	}
	for _, d := range deadends {
		identityID := getIdentityIDByEmail(d.email)
		if identityID == 0 {
			continue
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO risk_flags (identity_id, rule_key, severity, is_deadend, message, metadata, created_at)
			VALUES ($1, $2, $3, true, $4, $5::jsonb, $6)`,
			identityID, d.ruleKey, d.severity, d.message, d.meta, now,
		)
		if err != nil {
			log.Printf("warning: insert deadend flag for %s: %v", d.email, err)
		}
	}
	log.Println("deadend risk_flags seeded (SailPoint and cross-system with explanations)")

	fmt.Println("Mock connector run complete.")
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
