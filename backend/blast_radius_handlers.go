package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BlastRadiusNode represents a node in the blast radius graph
type BlastRadiusNode struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Type        string                 `json:"type"` // User, Group, Role, Permission, Resource
	Properties  map[string]interface{} `json:"properties"`
}

// BlastRadiusEdge represents an edge in the blast radius graph
type BlastRadiusEdge struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"` // MEMBER_OF, HAS_ROLE, PERMITS, etc.
	Label    string `json:"label"`
	Distance int    `json:"distance"` // Number of hops from source
}

// BlastRadiusResponse for GET /api/v1/graph/blast-radius/:id
type BlastRadiusResponse struct {
	IdentityID int64              `json:"identity_id"`
	MaxHops    int                `json:"max_hops"`
	Nodes      []BlastRadiusNode  `json:"nodes"`
	Edges      []BlastRadiusEdge  `json:"edges"`
	Stats      BlastRadiusStats   `json:"stats"`
}

type BlastRadiusStats struct {
	TotalNodes      int `json:"total_nodes"`
	TotalEdges      int `json:"total_edges"`
	GroupsReachable int `json:"groups_reachable"`
	RolesReachable  int `json:"roles_reachable"`
	PermissionsReachable int `json:"permissions_reachable"`
	ResourcesReachable   int `json:"resources_reachable"`
}

// GetBlastRadiusHandler computes blast radius for an identity using graph traversal
func GetBlastRadiusHandler(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		identityID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || identityID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}

		maxHops := 3 // Default
		if hops := c.Query("hops"); hops != "" {
			if parsed, err := strconv.Atoi(hops); err == nil && parsed > 0 && parsed <= 10 {
				maxHops = parsed
			}
		}

		ctx := c.Request.Context()

		// Verify identity exists in PostgreSQL
		var email string
		err = pool.QueryRow(ctx, `SELECT email FROM identities WHERE id = $1`, identityID).Scan(&email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "identity not found"})
			return
		}

		// Cypher query to find all reachable nodes within N hops
		query := `
			MATCH path = (start:User {id: $identity_id})-[*1..$max_hops]-(node)
			WHERE start.id = $identity_id
			WITH path, node, relationships(path) as rels
			RETURN DISTINCT
				node.id as node_id,
				labels(node)[0] as node_type,
				properties(node) as node_props,
				[startNode(r) | r IN rels][0] as start_node,
				[endNode(r) | r IN rels][0] as end_node,
				type(rels[0]) as edge_type,
				length(path) as distance
			ORDER BY distance, node_type
		`

		params := map[string]interface{}{
			"identity_id": identityID,
			"max_hops":    maxHops,
		}

		records, err := graphClient.ExecuteQuery(ctx, query, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "graph query failed", "details": err.Error()})
			return
		}

		// Build nodes and edges
		nodesMap := make(map[string]BlastRadiusNode)
		var edges []BlastRadiusEdge

		// Add source identity node
		nodesMap[fmt.Sprintf("user-%d", identityID)] = BlastRadiusNode{
			ID:   fmt.Sprintf("user-%d", identityID),
			Label: email,
			Type:  "User",
			Properties: map[string]interface{}{
				"id":    identityID,
				"email": email,
			},
		}

		edgeID := 0
		for _, record := range records {
			nodeID := fmt.Sprintf("%v", record["node_id"])
			nodeType := fmt.Sprintf("%v", record["node_type"])
			nodeProps, _ := record["node_props"].(map[string]interface{})
			distance, _ := record["distance"].(int64)

			// Create node
			label := ""
			if name, ok := nodeProps["name"].(string); ok {
				label = name
			} else if email, ok := nodeProps["email"].(string); ok {
				label = email
			} else {
				label = fmt.Sprintf("%s-%v", nodeType, nodeID)
			}

			nodeKey := fmt.Sprintf("%s-%v", nodeType, nodeID)
			nodesMap[nodeKey] = BlastRadiusNode{
				ID:         nodeKey,
				Label:      label,
				Type:       nodeType,
				Properties: nodeProps,
			}

			// Create edge
			edgeType := ""
			if et, ok := record["edge_type"].(string); ok {
				edgeType = et
			}

			sourceKey := fmt.Sprintf("user-%d", identityID)
			if distance > 1 {
				// Find intermediate nodes for multi-hop paths
				// For simplicity, we'll create direct edges
				sourceKey = nodeKey // Simplified - in production, track full path
			}

			edges = append(edges, BlastRadiusEdge{
				ID:       fmt.Sprintf("edge-%d", edgeID),
				Source:   sourceKey,
				Target:   nodeKey,
				Type:     edgeType,
				Label:    edgeType,
				Distance: int(distance),
			})
			edgeID++
		}

		// Convert nodes map to slice
		var nodes []BlastRadiusNode
		for _, node := range nodesMap {
			nodes = append(nodes, node)
		}

		// Calculate stats
		stats := BlastRadiusStats{
			TotalNodes: len(nodes),
			TotalEdges: len(edges),
		}
		for _, node := range nodes {
			switch node.Type {
			case "Group":
				stats.GroupsReachable++
			case "Role":
				stats.RolesReachable++
			case "Permission":
				stats.PermissionsReachable++
			case "Resource":
				stats.ResourcesReachable++
			}
		}

		c.JSON(http.StatusOK, BlastRadiusResponse{
			IdentityID: identityID,
			MaxHops:    maxHops,
			Nodes:      nodes,
			Edges:      edges,
			Stats:      stats,
		})
	}
}
