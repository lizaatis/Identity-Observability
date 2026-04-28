package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TimelineEvent represents a single event in the timeline
type TimelineEvent struct {
	EventID        int64                  `json:"event_id"`
	EventTime      string                 `json:"event_time"`
	SourceSystem   string                 `json:"source_system"`
	EventType      string                 `json:"event_type"`
	EventCategory  string                 `json:"event_category"`
	DisplayMessage *string                `json:"display_message,omitempty"`
	EventData      map[string]interface{} `json:"event_data"`
}

// TimelineResponse for GET /api/v1/identities/:id/timeline
type TimelineResponse struct {
	IdentityID int64           `json:"identity_id"`
	Events     []TimelineEvent  `json:"events"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
}

// RiskVelocityResponse for GET /api/v1/identities/:id/risk-velocity
type RiskVelocityResponse struct {
	IdentityID          int64                    `json:"identity_id"`
	CurrentWeekCount    int                      `json:"current_week_count"`
	PreviousWeekCount   int                      `json:"previous_week_count"`
	VelocityTrend       string                   `json:"velocity_trend"` // "increasing", "decreasing", "stable"
	WeeklyBreakdown     []WeeklyRiskVelocity     `json:"weekly_breakdown"`
}

type WeeklyRiskVelocity struct {
	WeekStart          string   `json:"week_start"`
	HighRiskEventCount int      `json:"high_risk_event_count"`
	EventTypes         []string `json:"event_types"`
	LastEventTime      string   `json:"last_event_time"`
}

// GetIdentityTimeline returns chronological events for an identity
func GetIdentityTimeline(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		identityID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || identityID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}

		// Pagination
		page := 1
		pageSize := 100
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}
		if ps := c.Query("page_size"); ps != "" {
			if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 500 {
				pageSize = parsed
			}
		}
		offset := (page - 1) * pageSize

		ctx := c.Request.Context()

		// Get total count
		var total int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) 
			FROM identity_events 
			WHERE identity_id = $1 AND processed = TRUE
		`, identityID).Scan(&total)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count events"})
			return
		}

		// Get events using the function
		rows, err := pool.Query(ctx, `
			SELECT * FROM get_identity_timeline($1, $2, $3)
		`, identityID, pageSize, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch timeline"})
			return
		}
		defer rows.Close()

		events := []TimelineEvent{}
		for rows.Next() {
			var event TimelineEvent
			var eventDataJSON []byte
			err := rows.Scan(
				&event.EventID,
				&event.EventTime,
				&event.SourceSystem,
				&event.EventType,
				&event.EventCategory,
				&event.DisplayMessage,
				&eventDataJSON,
			)
			if err != nil {
				continue
			}

			// Parse JSONB to map
			if len(eventDataJSON) > 0 {
				var eventData map[string]interface{}
				if err := json.Unmarshal(eventDataJSON, &eventData); err == nil {
					event.EventData = eventData
				} else {
					// Fallback to empty map if unmarshal fails
					event.EventData = make(map[string]interface{})
				}
			} else {
				event.EventData = make(map[string]interface{})
			}

			events = append(events, event)
		}

		c.JSON(http.StatusOK, TimelineResponse{
			IdentityID: identityID,
			Events:     events,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
		})
	}
}

// GetIdentityRiskVelocity returns risk velocity metrics for an identity
func GetIdentityRiskVelocity(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		identityID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || identityID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}

		ctx := c.Request.Context()

		// Get current week and previous week counts
		var currentWeekCount, previousWeekCount int
		err = pool.QueryRow(ctx, `
			WITH current_week AS (
				SELECT COUNT(*) as cnt
				FROM identity_risk_velocity
				WHERE identity_id = $1
				  AND week_start = DATE_TRUNC('week', NOW())
			),
			previous_week AS (
				SELECT COUNT(*) as cnt
				FROM identity_risk_velocity
				WHERE identity_id = $1
				  AND week_start = DATE_TRUNC('week', NOW()) - INTERVAL '1 week'
			)
			SELECT 
				COALESCE((SELECT cnt FROM current_week), 0),
				COALESCE((SELECT cnt FROM previous_week), 0)
		`, identityID).Scan(&currentWeekCount, &previousWeekCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to calculate risk velocity"})
			return
		}

		// Determine trend
		trend := "stable"
		if currentWeekCount > previousWeekCount {
			trend = "increasing"
		} else if currentWeekCount < previousWeekCount {
			trend = "decreasing"
		}

		// Get weekly breakdown (last 8 weeks)
		rows, err := pool.Query(ctx, `
			SELECT 
				week_start::text,
				high_risk_event_count,
				event_types,
				last_event_time::text
			FROM identity_risk_velocity
			WHERE identity_id = $1
			ORDER BY week_start DESC
			LIMIT 8
		`, identityID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch weekly breakdown"})
			return
		}
		defer rows.Close()

		weeklyBreakdown := []WeeklyRiskVelocity{}
		for rows.Next() {
			var week WeeklyRiskVelocity
			var eventTypes []string
			err := rows.Scan(
				&week.WeekStart,
				&week.HighRiskEventCount,
				&eventTypes,
				&week.LastEventTime,
			)
			if err != nil {
				continue
			}
			week.EventTypes = eventTypes
			weeklyBreakdown = append(weeklyBreakdown, week)
		}

		c.JSON(http.StatusOK, RiskVelocityResponse{
			IdentityID:        identityID,
			CurrentWeekCount:  currentWeekCount,
			PreviousWeekCount: previousWeekCount,
			VelocityTrend:     trend,
			WeeklyBreakdown:   weeklyBreakdown,
		})
	}
}
