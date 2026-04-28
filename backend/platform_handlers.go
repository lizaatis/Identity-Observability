package main

import (
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformDiagnostics explains environment health for empty/error UI and setup.
type PlatformDiagnostics struct {
	DatabaseOK           bool     `json:"database_ok"`
	IdentityEventsTable  bool     `json:"identity_events_table"`
	IdentitySourcesMeta  bool     `json:"identity_sources_metadata_column"`
	Neo4jConfigured      bool     `json:"neo4j_configured"`
	Neo4jReachable       bool     `json:"neo4j_reachable"`
	IdentityCount        int64    `json:"identity_count"`
	Messages             []string `json:"messages"`
}

// GraphStatusResponse for GET /api/v1/graph/status
type GraphStatusResponse struct {
	Neo4jReachable bool    `json:"neo4j_reachable"`
	LastSyncAt     *string `json:"last_sync_at,omitempty"`
	DurationMs     *int    `json:"duration_ms,omitempty"`
	NodeCount      *int    `json:"node_count,omitempty"`
	EdgeCount      *int    `json:"edge_count,omitempty"`
	LastError      *string `json:"last_error,omitempty"`
}

func GetPlatformDiagnostics(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		d := PlatformDiagnostics{Messages: []string{}}

		if err := pool.Ping(ctx); err != nil {
			d.Messages = append(d.Messages, "Database ping failed: "+err.Error())
			c.JSON(http.StatusOK, d)
			return
		}
		d.DatabaseOK = true

		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM identities`).Scan(&d.IdentityCount)

		var evExists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'identity_events'
			)
		`).Scan(&evExists); err == nil && evExists {
			d.IdentityEventsTable = true
		} else {
			d.Messages = append(d.Messages, "Table identity_events missing — run migration 006 for drift/timeline events.")
		}

		var metaExists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'identity_sources' AND column_name = 'metadata'
			)
		`).Scan(&metaExists); err == nil && metaExists {
			d.IdentitySourcesMeta = true
		} else {
			d.Messages = append(d.Messages, "Column identity_sources.metadata missing — run migration 008 for MFA and connector metadata.")
		}

		d.Neo4jConfigured = os.Getenv("NEO4J_URI") != ""
		if graphClient != nil {
			if err := graphClient.Ping(ctx); err == nil {
				d.Neo4jReachable = true
			} else {
				d.Messages = append(d.Messages, "Neo4j not reachable — blast radius and toxic combo graph queries need a running Neo4j (e.g. docker compose up neo4j).")
			}
		} else {
			d.Messages = append(d.Messages, "Neo4j client not initialized — graph features run in fallback mode.")
		}

		c.JSON(http.StatusOK, d)
	}
}

func GetGraphStatus(pool *pgxpool.Pool, graphClient *GraphClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		resp := GraphStatusResponse{}
		if graphClient != nil {
			if err := graphClient.Ping(ctx); err == nil {
				resp.Neo4jReachable = true
			}
		}
		var lastSync sql.NullTime
		var durMs, nodes, edges sql.NullInt64
		var lastErr sql.NullString
		err := pool.QueryRow(ctx, `
			SELECT last_sync_at, last_duration_ms, node_count, edge_count, last_error
			FROM graph_sync_state WHERE id = 1
		`).Scan(&lastSync, &durMs, &nodes, &edges, &lastErr)
		if err == nil {
			if lastSync.Valid {
				s := lastSync.Time.UTC().Format(time.RFC3339)
				resp.LastSyncAt = &s
			}
			if durMs.Valid {
				v := int(durMs.Int64)
				resp.DurationMs = &v
			}
			if nodes.Valid {
				v := int(nodes.Int64)
				resp.NodeCount = &v
			}
			if edges.Valid {
				v := int(edges.Int64)
				resp.EdgeCount = &v
			}
			if lastErr.Valid {
				s := lastErr.String
				resp.LastError = &s
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}
