package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Default matches docker-compose: postgres exposed on 5434
		dbURL = "postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	r := gin.Default()
	r.SetTrustedProxies(nil) // do not trust any proxy; set to specific IPs if behind a reverse proxy

	// Optional Basic Auth (when AUTH_USER and AUTH_PASSWORD are set)
	r.Use(BasicAuthMiddleware())

	// Initialize risk engine
	mainIdPKey := os.Getenv("MAIN_IDP_KEY")
	if mainIdPKey == "" {
		mainIdPKey = "okta_primary" // default
	}
	riskEngine := NewRiskEngine(pool, mainIdPKey)

	// Initialize event queue (Redis Streams if available, otherwise in-memory)
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_PORT")
		if redisURL != "" {
			redisURL = "localhost:" + redisURL
		} else {
			redisURL = "localhost:6379" // Default Redis port
		}
	}

	eventQueue, err := NewRedisEventQueue(redisURL, "identity_events", "event_processors")
	if err != nil {
		// Fallback to in-memory queue if Redis unavailable
		log.Printf("Warning: Redis unavailable, using in-memory queue: %v", err)
		eventQueue = NewInMemoryEventQueue()
	} else {
		log.Println("Using Redis Streams for event queue")
	}

	// Initialize webhook handler and event processor
	webhookHandler := NewWebhookHandler(pool, eventQueue)
	ctx := context.Background()
	webhookHandler.StartEventProcessor(ctx, "event_processors", "processor-1")

	// Initialize Neo4j graph client
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "bolt://localhost:7687"
	}
	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPassword := os.Getenv("NEO4J_PASSWORD")
	if neo4jPassword == "" {
		neo4jPassword = "observability_dev"
	}

	var graphClient *GraphClient
	var graphSync *GraphSync
	var toxicComboEngine *ToxicComboEngine
	var iqlParser *IQLParser

	if gc, err := NewGraphClient(neo4jURI, neo4jUser, neo4jPassword); err == nil {
		graphClient = gc
		graphSync = NewGraphSync(pool, graphClient)
		toxicComboEngine, _ = NewToxicComboEngine(graphClient, os.Getenv("TOXIC_COMBO_RULES_PATH"))
		iqlParser = NewIQLParser(pool, graphClient)
		log.Println("Neo4j graph client initialized")
	} else {
		log.Printf("Warning: Neo4j unavailable, graph features disabled: %v", err)
	}

	// Health check (Postgres + Neo4j)
	r.GET("/health", healthCheck(pool, graphClient))

	// Platform diagnostics & graph visibility (Postgres source of truth + Neo4j paths)
	r.GET("/api/v1/platform/diagnostics", GetPlatformDiagnostics(pool, graphClient))
	r.GET("/api/v1/graph/status", GetGraphStatus(pool, graphClient))

	// Identity endpoints
	r.GET("/api/v1/identities", listIdentities(pool))
	r.GET("/api/v1/identities/:id", identityByID(pool))
	r.GET("/api/v1/identities/:id/effective-permissions", getEffectivePermissions(pool))
	r.GET("/api/v1/identities/:id/timeline", GetIdentityTimeline(pool))
	r.GET("/api/v1/identities/:id/risk-velocity", GetIdentityRiskVelocity(pool))

	// Risk endpoints
	r.GET("/api/v1/identities/:id/risk", getIdentityRisk(pool, riskEngine))
	r.GET("/api/v1/risk/top", getTopRiskyIdentities(pool))

	// Dashboard endpoints
	r.GET("/api/v1/dashboard/stats", getDashboardStats(pool))

	// Changes (drift) endpoint
	r.GET("/api/v1/changes", GetChanges(pool))

	// Cross-system lenses
	r.GET("/api/v1/lenses/privileged", GetLensPrivileged(pool))
	r.GET("/api/v1/lenses/cross-cloud-admins", GetLensCrossCloudAdmins(pool))
	r.GET("/api/v1/lenses/deadends", GetLensDeadends(pool))
	r.GET("/api/v1/lenses/no-mfa", GetLensNoMfa(pool))

	// Connector endpoints
	r.GET("/api/v1/connectors", listConnectors(pool))
	r.GET("/api/v1/connectors/full-status", listConnectorsFull(pool))
	r.GET("/api/v1/connectors/:id/status", getConnectorStatus(pool))
	r.POST("/api/v1/connectors/test/okta", RequireAdmin(), TestOktaConnection())
	r.POST("/api/v1/connectors/test/sailpoint", RequireAdmin(), TestSailPointConnection())
	r.POST("/api/v1/connectors/test/gcp", RequireAdmin(), TestGCPConnection())

	// Export endpoints (blocked for read_only when auth is on)
	r.GET("/api/v1/export/identities/:id", RequireNotReadOnly(), exportIdentity(pool))
	r.GET("/api/v1/export/risk/high-risk", RequireNotReadOnly(), exportHighRisk(pool))
	r.GET("/api/v1/export/risk/deadends", RequireNotReadOnly(), exportDeadends(pool))

	// Stitching review queue (ambiguous correlation — before ML)
	r.GET("/api/v1/stitching/review", ListStitchingReview(pool))

	// Alerts (PagerDuty/Tines webhook)
	r.POST("/api/v1/alerts/emit", EmitAlerts(pool))

	// Webhook endpoints (real-time event ingestion)
	r.POST("/api/v1/webhooks/okta", webhookHandler.HandleOktaWebhook())

	// Graph endpoints
	if graphClient != nil {
		r.GET("/api/v1/graph/blast-radius/:id", GetBlastRadiusHandler(pool, graphClient))
		r.POST("/api/v1/graph/sync", SyncGraph(pool, graphSync))
		if toxicComboEngine != nil {
			r.GET("/api/v1/graph/toxic-combo", GetToxicComboMatches(toxicComboEngine))
		} else {
			// Fallback handler when toxic combo engine is unavailable
			r.GET("/api/v1/graph/toxic-combo", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"matches": []ToxicComboMatch{},
					"count":   0,
					"message": "Toxic combo engine unavailable - Neo4j may not be connected",
				})
			})
		}
		if iqlParser != nil {
			r.GET("/api/v1/iql/search", ExecuteIQL(iqlParser))
		}
	} else {
		// Fallback handlers when Neo4j is unavailable
		r.GET("/api/v1/graph/toxic-combo", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"matches": []ToxicComboMatch{},
				"count":   0,
				"message": "Graph features unavailable - Neo4j not connected",
			})
		})
		// Blast radius: return empty graph with message so UI doesn't show "Failed to load"
		r.GET("/api/v1/graph/blast-radius/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			identityID, _ := strconv.ParseInt(idStr, 10, 64)
			if identityID <= 0 {
				identityID = 0
			}
			c.JSON(http.StatusOK, gin.H{
				"identity_id": identityID,
				"max_hops":    3,
				"nodes":       []BlastRadiusNode{},
				"edges":       []BlastRadiusEdge{},
				"stats": gin.H{
					"total_nodes":            0,
					"total_edges":            0,
					"groups_reachable":       0,
					"roles_reachable":        0,
					"permissions_reachable":  0,
					"resources_reachable":    0,
				},
				"message": "Blast Radius requires Neo4j. Start Neo4j (e.g. docker compose up -d neo4j), then run Sync Graph to populate the graph.",
			})
		})
		r.POST("/api/v1/graph/sync", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "skipped", "message": "Neo4j not connected"})
		})
	}
	
	// IQL fields endpoint (works with or without Neo4j)
	if iqlParser != nil {
		r.GET("/api/v1/iql/fields", GetIQLFields(iqlParser))
	} else {
		// Fallback handler when Neo4j is unavailable
		r.GET("/api/v1/iql/fields", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"fields": []string{"system", "mfa", "role", "status", "email", "group"},
				"message": "Graph features unavailable - using basic fields only",
				"examples": []string{
					"system:aws mfa:false",
					"role:admin status:active",
					"email:alice@company.com",
				},
			})
		})
	}

	// Initialize action hub clients
	slackClient := NewSlackClient()
	jiraClient := NewJiraClient()

	// Remediation/Action endpoints (Admin only)
	r.POST("/api/v1/identities/:id/remediate", RequireAdmin(), CreateRemediationAction(pool, slackClient, jiraClient))
	r.POST("/api/v1/remediation/:id/approve", RequireAdmin(), ApproveRemediationAction(pool))
	r.POST("/api/v1/remediation/:id/reject", RejectRemediationAction(pool))
	if slackClient != nil {
		r.POST("/api/v1/slack/interactive", slackClient.HandleSlackInteraction(pool))
	}

	// Executive Dashboard
	r.GET("/api/v1/dashboard/executive", GetExecutiveDashboard(pool))

	// Custom Rules
	if graphClient != nil {
		r.GET("/api/v1/custom-rules", ListCustomRules(pool))
		r.POST("/api/v1/custom-rules", CreateCustomRule(pool, graphClient))
		r.POST("/api/v1/custom-rules/:id/test", TestCustomRule(pool, graphClient))
	}

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	log.Printf("listening on %s", addr)
	log.Fatal(r.Run(":" + addr))
}
