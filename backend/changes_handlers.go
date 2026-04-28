package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChangeItem represents one entry in the changes feed
type ChangeItem struct {
	ID           int64    `json:"id"`
	EventTime    string   `json:"event_time"`
	SourceSystem string   `json:"source_system"`
	EventType    string   `json:"event_type"`
	IdentityID   *int64   `json:"identity_id,omitempty"`
	Email        *string  `json:"email,omitempty"`
	DisplayMsg   *string  `json:"display_message,omitempty"`
	Summary      string   `json:"summary"` // e.g. "User X gained Okta Super Admin"
}

// GetChanges returns recent identity changes (drift).
// GET /api/v1/changes?since=24h
// GET /api/v1/changes?identity=:id
func GetChanges(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		since := c.Query("since") // e.g. 24h, 7d
		identityIDStr := c.Query("identity")

		var sinceTime time.Time
		if since != "" {
			d, err := parseDuration(since)
			if err != nil {
				sinceTime = time.Now().Add(-24 * time.Hour)
			} else {
				sinceTime = time.Now().Add(-d)
			}
		} else {
			sinceTime = time.Now().Add(-24 * time.Hour)
		}

		// Build query: identity_events (or fallback to risk_flags/risk_score_history if events empty)
		query := `
			SELECT ie.id, ie.event_time::text, ie.source_system, ie.event_type,
			       ie.identity_id, i.email, i.display_name,
			       ie.event_data->>'displayMessage' AS display_message
			FROM identity_events ie
			LEFT JOIN identities i ON i.id = ie.identity_id
			WHERE ie.event_time >= $1`
		args := []interface{}{sinceTime}

		if identityIDStr != "" {
			id, err := strconv.ParseInt(identityIDStr, 10, 64)
			if err == nil && id > 0 {
				query += ` AND ie.identity_id = $2`
				args = append(args, id)
			}
		}

		query += ` ORDER BY ie.event_time DESC LIMIT 100`

		rows, err := pool.Query(ctx, query, args...)
		if err != nil {
			// Table might not exist (TimescaleDB not run)
			c.JSON(http.StatusOK, gin.H{"changes": []ChangeItem{}})
			return
		}
		defer rows.Close()

		var changes []ChangeItem
		for rows.Next() {
			var item ChangeItem
			var email, displayName *string
			if err := rows.Scan(&item.ID, &item.EventTime, &item.SourceSystem, &item.EventType,
				&item.IdentityID, &email, &displayName, &item.DisplayMsg); err != nil {
				continue
			}
			item.Email = email
			if item.DisplayMsg != nil && *item.DisplayMsg != "" {
				item.Summary = *item.DisplayMsg
			} else {
				item.Summary = formatChangeSummary(item.SourceSystem, item.EventType, email, displayName)
			}
			changes = append(changes, item)
		}

		c.JSON(http.StatusOK, gin.H{"changes": changes})
	}
}

func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, strconv.ErrSyntax
	}
	// Allow "24h" or "7d" style
	num, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, err
	}
	switch s[len(s)-1] {
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return time.Duration(num) * time.Hour, nil
	}
}

func formatChangeSummary(source, eventType string, email, displayName *string) string {
	name := "Unknown"
	if displayName != nil && *displayName != "" {
		name = *displayName
	} else if email != nil && *email != "" {
		name = *email
	}
	// Human-readable: "User X gained Okta Super Admin" style
	return name + " – " + source + " " + eventType
}
