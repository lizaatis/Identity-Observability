package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetToxicComboMatches returns matches for all toxic combo rules
func GetToxicComboMatches(engine *ToxicComboEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		if engine == nil {
			c.JSON(http.StatusOK, gin.H{
				"matches": []ToxicComboMatch{},
				"count":   0,
				"message": "Toxic combo engine not initialized",
			})
			return
		}

		ctx := c.Request.Context()

		matches, err := engine.EvaluateAll(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to evaluate rules", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"matches": matches,
			"count":   len(matches),
		})
	}
}

// SyncGraph syncs PostgreSQL data to Neo4j
func SyncGraph(pool *pgxpool.Pool, graphSync *GraphSync) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		start := time.Now()
		err := graphSync.SyncAll(ctx)
		durMs := int(time.Since(start).Milliseconds())
		if recErr := RecordGraphSyncState(ctx, pool, graphSync.graphClient, durMs, err); recErr != nil {
			// Non-fatal: migration 009 may not be applied yet
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "graph sync failed", "details": err.Error(), "duration_ms": durMs})
			return
		}
		var nodes, edges int64
		if graphSync.graphClient != nil {
			n, e, cerr := CountGraphNodesEdges(ctx, graphSync.graphClient)
			if cerr == nil {
				nodes, edges = n, e
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"status":       "success",
			"message":      "Graph synced successfully",
			"duration_ms":  durMs,
			"node_count":   nodes,
			"edge_count":   edges,
		})
	}
}

// ExecuteIQL executes an IQL query
func ExecuteIQL(parser *IQLParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		queryStr := c.Query("q")
		if queryStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
			return
		}

		query, err := parser.Parse(queryStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query", "details": err.Error()})
			return
		}

		ctx := c.Request.Context()
		results, err := parser.Execute(ctx, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query execution failed", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query.Raw,
			"results": results,
			"count":   len(results),
		})
	}
}

// GetIQLFields returns supported IQL fields
func GetIQLFields(parser *IQLParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		fields := parser.GetSupportedFields()
		c.JSON(http.StatusOK, gin.H{
			"fields": fields,
			"examples": []string{
				"system:aws mfa:false",
				"role:admin status:active",
				"email:alice@company.com",
				"group:engineering",
			},
		})
	}
}
