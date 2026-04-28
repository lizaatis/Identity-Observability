package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GraphSync syncs PostgreSQL relationship data to Neo4j
type GraphSync struct {
	pool        *pgxpool.Pool
	graphClient *GraphClient
}

// NewGraphSync creates a new graph sync service
func NewGraphSync(pool *pgxpool.Pool, graphClient *GraphClient) *GraphSync {
	return &GraphSync{
		pool:        pool,
		graphClient: graphClient,
	}
}

// SyncAll syncs all relationships from PostgreSQL to Neo4j
func (gs *GraphSync) SyncAll(ctx context.Context) error {
	log.Println("Starting graph sync from PostgreSQL to Neo4j...")

	// Clear existing graph
	if err := gs.clearGraph(ctx); err != nil {
		return fmt.Errorf("clear graph: %w", err)
	}

	// Sync nodes
	if err := gs.syncNodes(ctx); err != nil {
		return fmt.Errorf("sync nodes: %w", err)
	}

	// Sync relationships
	if err := gs.syncRelationships(ctx); err != nil {
		return fmt.Errorf("sync relationships: %w", err)
	}

	log.Println("Graph sync completed successfully")
	return nil
}

// CountGraphNodesEdges returns node and relationship counts in Neo4j.
func CountGraphNodesEdges(ctx context.Context, gc *GraphClient) (nodes int64, edges int64, err error) {
	rn, err := gc.ExecuteQuery(ctx, "MATCH (n) RETURN count(n) AS c", nil)
	if err != nil {
		return 0, 0, err
	}
	if len(rn) > 0 {
		nodes = neo4jInt64(rn[0]["c"])
	}
	re, err := gc.ExecuteQuery(ctx, "MATCH ()-[r]->() RETURN count(r) AS c", nil)
	if err != nil {
		return nodes, 0, err
	}
	if len(re) > 0 {
		edges = neo4jInt64(re[0]["c"])
	}
	return nodes, edges, nil
}

func neo4jInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	default:
		return 0
	}
}

// RecordGraphSyncState persists last sync outcome, counts, and errors for visibility in the UI.
func RecordGraphSyncState(ctx context.Context, pool *pgxpool.Pool, gc *GraphClient, durationMs int, syncErr error) error {
	var lastErr *string
	if syncErr != nil {
		s := syncErr.Error()
		lastErr = &s
	}
	var nodeCount, edgeCount *int
	if syncErr == nil && gc != nil {
		n, e, err := CountGraphNodesEdges(ctx, gc)
		if err == nil {
			ni, ei := int(n), int(e)
			nodeCount, edgeCount = &ni, &ei
		}
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO graph_sync_state (id, last_sync_at, last_duration_ms, node_count, edge_count, last_error, updated_at)
		VALUES (1, NOW(), $1, $2, $3, $4, NOW())
		ON CONFLICT (id) DO UPDATE SET
			last_sync_at = EXCLUDED.last_sync_at,
			last_duration_ms = EXCLUDED.last_duration_ms,
			last_error = EXCLUDED.last_error,
			updated_at = NOW(),
			node_count = CASE WHEN EXCLUDED.last_error IS NULL THEN COALESCE(EXCLUDED.node_count, graph_sync_state.node_count) ELSE graph_sync_state.node_count END,
			edge_count = CASE WHEN EXCLUDED.last_error IS NULL THEN COALESCE(EXCLUDED.edge_count, graph_sync_state.edge_count) ELSE graph_sync_state.edge_count END
	`, durationMs, nodeCount, edgeCount, lastErr)
	return err
}

// clearGraph removes all nodes and relationships
func (gs *GraphSync) clearGraph(ctx context.Context) error {
	query := "MATCH (n) DETACH DELETE n"
	return gs.graphClient.ExecuteWrite(ctx, query, nil)
}

// syncNodes creates nodes for identities, groups, roles, permissions, resources
func (gs *GraphSync) syncNodes(ctx context.Context) error {
	// Sync Identities
	query := `
		UNWIND $identities AS identity
		MERGE (u:User {id: identity.id})
		SET u.email = identity.email,
		    u.display_name = identity.display_name,
		    u.employee_id = identity.employee_id,
		    u.status = identity.status
	`
	identities, err := gs.fetchIdentities(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"identities": identities}); err != nil {
		return fmt.Errorf("sync identities: %w", err)
	}

	// Sync Groups
	query = `
		UNWIND $groups AS group
		MERGE (g:Group {id: group.id})
		SET g.name = group.name,
		    g.source_system = group.source_system
	`
	groups, err := gs.fetchGroups(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"groups": groups}); err != nil {
		return fmt.Errorf("sync groups: %w", err)
	}

	// Sync Roles
	query = `
		UNWIND $roles AS role
		MERGE (r:Role {id: role.id})
		SET r.name = role.name,
		    r.privilege_level = role.privilege_level,
		    r.source_system = role.source_system,
		    r.admin = (role.privilege_level = 'admin')
	`
	roles, err := gs.fetchRoles(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"roles": roles}); err != nil {
		return fmt.Errorf("sync roles: %w", err)
	}

	// Sync Permissions
	query = `
		UNWIND $permissions AS perm
		MERGE (p:Permission {id: perm.id})
		SET p.name = perm.name,
		    p.resource_type = perm.resource_type,
		    p.source_system = perm.source_system
	`
	permissions, err := gs.fetchPermissions(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"permissions": permissions}); err != nil {
		return fmt.Errorf("sync permissions: %w", err)
	}

	// Sync Resources
	query = `
		UNWIND $resources AS resource
		MERGE (res:Resource {id: resource.id})
		SET res.name = resource.name,
		    res.resource_type = resource.resource_type,
		    res.source_system = resource.source_system
	`
	resources, err := gs.fetchResources(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"resources": resources}); err != nil {
		return fmt.Errorf("sync resources: %w", err)
	}

	return nil
}

// syncRelationships creates edges between nodes
func (gs *GraphSync) syncRelationships(ctx context.Context) error {
	// Identity -> Group (MEMBER_OF)
	query := `
		UNWIND $relationships AS rel
		MATCH (u:User {id: rel.identity_id})
		MATCH (g:Group {id: rel.group_id})
		MERGE (u)-[:MEMBER_OF {source_system: rel.source_system}]->(g)
	`
	identityGroups, err := gs.fetchIdentityGroups(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"relationships": identityGroups}); err != nil {
		return fmt.Errorf("sync identity_group: %w", err)
	}

	// Identity -> Role (HAS_ROLE)
	query = `
		UNWIND $relationships AS rel
		MATCH (u:User {id: rel.identity_id})
		MATCH (r:Role {id: rel.role_id})
		MERGE (u)-[:HAS_ROLE {source_system: rel.source_system}]->(r)
	`
	identityRoles, err := gs.fetchIdentityRoles(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"relationships": identityRoles}); err != nil {
		return fmt.Errorf("sync identity_role: %w", err)
	}

	// Group -> Role (HAS_ROLE)
	query = `
		UNWIND $relationships AS rel
		MATCH (g:Group {id: rel.group_id})
		MATCH (r:Role {id: rel.role_id})
		MERGE (g)-[:HAS_ROLE {source_system: rel.source_system}]->(r)
	`
	groupRoles, err := gs.fetchGroupRoles(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"relationships": groupRoles}); err != nil {
		return fmt.Errorf("sync group_role: %w", err)
	}

	// Role -> Permission (PERMITS)
	query = `
		UNWIND $relationships AS rel
		MATCH (r:Role {id: rel.role_id})
		MATCH (p:Permission {id: rel.permission_id})
		MERGE (r)-[:PERMITS {source_system: rel.source_system}]->(p)
	`
	rolePermissions, err := gs.fetchRolePermissions(ctx)
	if err != nil {
		return err
	}
	if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"relationships": rolePermissions}); err != nil {
		return fmt.Errorf("sync role_permission: %w", err)
	}

	// Permission -> Resource (if linked)
	query = `
		UNWIND $relationships AS rel
		MATCH (p:Permission {id: rel.permission_id})
		MATCH (res:Resource {id: rel.resource_id})
		MERGE (p)-[:ON_RESOURCE]->(res)
	`
	permissionResources, err := gs.fetchPermissionResources(ctx)
	if err != nil {
		return err
	}
	if len(permissionResources) > 0 {
		if err := gs.graphClient.ExecuteWrite(ctx, query, map[string]interface{}{"relationships": permissionResources}); err != nil {
			return fmt.Errorf("sync permission_resource: %w", err)
		}
	}

	return nil
}

// Fetch functions
func (gs *GraphSync) fetchIdentities(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `
		SELECT id, email, display_name, employee_id, status
		FROM identities
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []map[string]interface{}
	for rows.Next() {
		var id int64
		var email, displayName, employeeID, status string
		if err := rows.Scan(&id, &email, &displayName, &employeeID, &status); err != nil {
			continue
		}
		identities = append(identities, map[string]interface{}{
			"id":          id,
			"email":       email,
			"display_name": displayName,
			"employee_id": employeeID,
			"status":      status,
		})
	}
	return identities, nil
}

func (gs *GraphSync) fetchGroups(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT id, name, source_system FROM groups`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, sourceSystem string
		if err := rows.Scan(&id, &name, &sourceSystem); err != nil {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"id":           id,
			"name":         name,
			"source_system": sourceSystem,
		})
	}
	return groups, nil
}

func (gs *GraphSync) fetchRoles(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT id, name, privilege_level, source_system FROM roles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, privilegeLevel, sourceSystem string
		if err := rows.Scan(&id, &name, &privilegeLevel, &sourceSystem); err != nil {
			continue
		}
		roles = append(roles, map[string]interface{}{
			"id":              id,
			"name":             name,
			"privilege_level": privilegeLevel,
			"source_system":    sourceSystem,
		})
	}
	return roles, nil
}

func (gs *GraphSync) fetchPermissions(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT id, name, resource_type, source_system FROM permissions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, resourceType, sourceSystem string
		if err := rows.Scan(&id, &name, &resourceType, &sourceSystem); err != nil {
			continue
		}
		permissions = append(permissions, map[string]interface{}{
			"id":            id,
			"name":          name,
			"resource_type": resourceType,
			"source_system": sourceSystem,
		})
	}
	return permissions, nil
}

func (gs *GraphSync) fetchResources(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT id, name, resource_type, source_system FROM resources`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var resources []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, resourceType, sourceSystem string
		if err := rows.Scan(&id, &name, &resourceType, &sourceSystem); err != nil {
			continue
		}
		resources = append(resources, map[string]interface{}{
			"id":            id,
			"name":          name,
			"resource_type": resourceType,
			"source_system": sourceSystem,
		})
	}
	return resources, nil
}

func (gs *GraphSync) fetchIdentityGroups(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT identity_id, group_id, source_system FROM identity_group`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []map[string]interface{}
	for rows.Next() {
		var identityID, groupID int64
		var sourceSystem string
		if err := rows.Scan(&identityID, &groupID, &sourceSystem); err != nil {
			continue
		}
		rels = append(rels, map[string]interface{}{
			"identity_id":   identityID,
			"group_id":      groupID,
			"source_system": sourceSystem,
		})
	}
	return rels, nil
}

func (gs *GraphSync) fetchIdentityRoles(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT identity_id, role_id, source_system FROM identity_role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []map[string]interface{}
	for rows.Next() {
		var identityID, roleID int64
		var sourceSystem string
		if err := rows.Scan(&identityID, &roleID, &sourceSystem); err != nil {
			continue
		}
		rels = append(rels, map[string]interface{}{
			"identity_id":   identityID,
			"role_id":       roleID,
			"source_system": sourceSystem,
		})
	}
	return rels, nil
}

func (gs *GraphSync) fetchGroupRoles(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT group_id, role_id, source_system FROM group_role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []map[string]interface{}
	for rows.Next() {
		var groupID, roleID int64
		var sourceSystem string
		if err := rows.Scan(&groupID, &roleID, &sourceSystem); err != nil {
			continue
		}
		rels = append(rels, map[string]interface{}{
			"group_id":      groupID,
			"role_id":       roleID,
			"source_system": sourceSystem,
		})
	}
	return rels, nil
}

func (gs *GraphSync) fetchRolePermissions(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `SELECT role_id, permission_id, source_system FROM role_permission`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []map[string]interface{}
	for rows.Next() {
		var roleID, permissionID int64
		var sourceSystem string
		if err := rows.Scan(&roleID, &permissionID, &sourceSystem); err != nil {
			continue
		}
		rels = append(rels, map[string]interface{}{
			"role_id":       roleID,
			"permission_id": permissionID,
			"source_system": sourceSystem,
		})
	}
	return rels, nil
}

func (gs *GraphSync) fetchPermissionResources(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := gs.pool.Query(ctx, `
		SELECT permission_id, resource_id
		FROM permissions
		WHERE resource_id IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []map[string]interface{}
	for rows.Next() {
		var permissionID, resourceID int64
		if err := rows.Scan(&permissionID, &resourceID); err != nil {
			continue
		}
		rels = append(rels, map[string]interface{}{
			"permission_id": permissionID,
			"resource_id":   resourceID,
		})
	}
	return rels, nil
}
