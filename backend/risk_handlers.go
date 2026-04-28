package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RiskResponse for GET /api/v1/identities/:id/risk
type RiskResponse struct {
	IdentityID  int64         `json:"identity_id"`
	Score       int           `json:"score"`
	MaxSeverity string        `json:"max_severity"`
	ComputedAt  string        `json:"computed_at"`
	Flags       []RiskFlagDTO `json:"flags"`
}

// RiskFlagDTO for API response
type RiskFlagDTO struct {
	RuleKey   string                 `json:"rule_key"`
	Severity  string                 `json:"severity"`
	IsDeadend bool                   `json:"is_deadend"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TopRiskResponse for GET /api/v1/risk/top
type TopRiskResponse struct {
	Identities []TopRiskIdentityDTO `json:"identities"`
	Limit      int                  `json:"limit"`
}

type TopRiskIdentityDTO struct {
	IdentityID   int64    `json:"identity_id"`
	Email        string   `json:"email"`
	DisplayName  *string   `json:"display_name,omitempty"`
	Score        int      `json:"score"`
	MaxSeverity  string   `json:"max_severity"`
	FlagCount    int      `json:"flag_count"`
	SourceSystems []string `json:"source_systems"`
}

// Get identity risk score and flags
func getIdentityRisk(pool *pgxpool.Pool, engine *RiskEngine) gin.HandlerFunc {
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

		// Check if we should recompute or use cached
		recompute := c.Query("recompute") == "true"

		var riskScore *RiskScore
		if recompute {
			// Compute fresh risk score
			riskScore, err = engine.ComputeRiskScore(ctx, id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// Store it
			if err := engine.StoreRiskScore(ctx, riskScore); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			// Try to get from database
			var score int
			var maxSeverity string
			var computedAt time.Time
			var detailJSON []byte

			err = pool.QueryRow(ctx, `
				SELECT score, max_severity, computed_at, detail
				FROM risk_scores
				WHERE identity_id = $1
			`, id).Scan(&score, &maxSeverity, &computedAt, &detailJSON)

			if err == pgx.ErrNoRows {
				// No cached score, compute it
				riskScore, err = engine.ComputeRiskScore(ctx, id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				// Store it
				if err := engine.StoreRiskScore(ctx, riskScore); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			} else {
				// Load flags from database
				rows, err := pool.Query(ctx, `
					SELECT rule_key, severity, is_deadend, message, metadata
					FROM risk_flags
					WHERE identity_id = $1 AND cleared_at IS NULL
					ORDER BY severity DESC, rule_key
				`, id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				defer rows.Close()

				var flags []RiskFlagDTO
				for rows.Next() {
					var flag RiskFlagDTO
					var metadataJSON []byte
					if err := rows.Scan(&flag.RuleKey, &flag.Severity, &flag.IsDeadend, &flag.Message, &metadataJSON); err != nil {
						continue
					}
					if len(metadataJSON) > 0 {
						json.Unmarshal(metadataJSON, &flag.Metadata)
					}
					flags = append(flags, flag)
				}

				riskScore = &RiskScore{
					IdentityID:  id,
					Score:       score,
					MaxSeverity: maxSeverity,
					ComputedAt:  computedAt,
					Flags:       convertFlags(flags),
				}
			}
		}

		// Convert to response
		flags := make([]RiskFlagDTO, len(riskScore.Flags))
		for i, f := range riskScore.Flags {
			flags[i] = RiskFlagDTO{
				RuleKey:   f.RuleKey,
				Severity:  f.Severity,
				IsDeadend: f.IsDeadend,
				Message:   f.Message,
				Metadata:  f.Metadata,
			}
		}

		c.JSON(http.StatusOK, RiskResponse{
			IdentityID:  riskScore.IdentityID,
			Score:       riskScore.Score,
			MaxSeverity: riskScore.MaxSeverity,
			ComputedAt:  riskScore.ComputedAt.Format("2006-01-02T15:04:05Z"),
			Flags:       flags,
		})
	}
}

// Get top risky identities
func getTopRiskyIdentities(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		limit := 10
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}

		rows, err := pool.Query(ctx, `
			SELECT 
				rs.identity_id,
				i.email,
				i.display_name,
				rs.score,
				rs.max_severity,
				COUNT(DISTINCT rf.id) as flag_count,
				ARRAY_AGG(DISTINCT is2.source_system) FILTER (WHERE is2.source_system IS NOT NULL) as source_systems
			FROM risk_scores rs
			JOIN identities i ON i.id = rs.identity_id
			LEFT JOIN risk_flags rf ON rf.identity_id = rs.identity_id AND rf.cleared_at IS NULL
			LEFT JOIN identity_sources is2 ON is2.identity_id = rs.identity_id
			GROUP BY rs.identity_id, i.email, i.display_name, rs.score, rs.max_severity
			ORDER BY rs.score DESC, rs.max_severity DESC
			LIMIT $1
		`, limit)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var identities []TopRiskIdentityDTO
		for rows.Next() {
			var ident TopRiskIdentityDTO
			var sourceSystems []string
			if err := rows.Scan(&ident.IdentityID, &ident.Email, &ident.DisplayName, &ident.Score, &ident.MaxSeverity, &ident.FlagCount, &sourceSystems); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if sourceSystems == nil {
				sourceSystems = []string{}
			}
			ident.SourceSystems = sourceSystems
			identities = append(identities, ident)
		}

		c.JSON(http.StatusOK, TopRiskResponse{
			Identities: identities,
			Limit:      limit,
		})
	}
}

// Helper to convert RiskFlagDTO to RiskFlag
func convertFlags(dtos []RiskFlagDTO) []RiskFlag {
	flags := make([]RiskFlag, len(dtos))
	for i, dto := range dtos {
		flags[i] = RiskFlag{
			RuleKey:   dto.RuleKey,
			Severity:  dto.Severity,
			IsDeadend: dto.IsDeadend,
			Message:   dto.Message,
			Metadata:  dto.Metadata,
		}
	}
	return flags
}
