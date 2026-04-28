package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// High-value triggers that should emit to PagerDuty/Tines
var highValueRuleKeys = map[string]bool{
	"deadend_orphaned_user": true,
	"super_admin":           true,
	"cross_system_admin":     true,
	"org_owner":              true,
}

// AlertPayload is the JSON body POSTed to ALERT_WEBHOOK_URL for high-value risk triggers.
// Dedupe: table alert_webhook_dedupe (migration 009) stores last send per (identity_id, rule_key); window ALERT_DEDUPE_HOURS (default 24).
type AlertPayload struct {
	Trigger     string   `json:"trigger"`
	IdentityID  int64    `json:"identity_id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name,omitempty"`
	Systems     []string `json:"systems"`
	DeepLink    string   `json:"deep_link"`
	Severity    string   `json:"severity"`
	Message     string   `json:"message"`
	Timestamp   string   `json:"timestamp"`
}

// NotifyHighValueTriggers queries risk_flags since the given time for high-value rules,
// and POSTs each to ALERT_WEBHOOK_URL (if set).
func NotifyHighValueTriggers(ctx context.Context, pool *pgxpool.Pool, since time.Time, appBaseURL string) (sent int, err error) {
	webhookURL := os.Getenv("ALERT_WEBHOOK_URL")
	if webhookURL == "" {
		return 0, nil
	}
	if appBaseURL == "" {
		appBaseURL = os.Getenv("APP_BASE_URL")
		if appBaseURL == "" {
			appBaseURL = "http://localhost:5173"
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT rf.identity_id, rf.rule_key, rf.severity, rf.message,
		       i.email, i.display_name,
		       COALESCE((SELECT array_agg(DISTINCT source_system) FROM identity_sources WHERE identity_id = rf.identity_id), ARRAY[]::text[]) as systems
		FROM risk_flags rf
		JOIN identities i ON i.id = rf.identity_id
		WHERE rf.rule_key IN ('deadend_orphaned_user', 'super_admin', 'cross_system_admin', 'org_owner')
		  AND rf.cleared_at IS NULL AND rf.created_at >= $1
	`, since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	dedupeHours := 24
	if v := os.Getenv("ALERT_DEDUPE_HOURS"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			dedupeHours = h
		}
	}
	dedupeWindow := time.Duration(dedupeHours) * time.Hour

	client := &http.Client{Timeout: 10 * time.Second}
	for rows.Next() {
		var identityID int64
		var ruleKey, severity, message, email string
		var displayName *string
		var systems []string
		if rows.Scan(&identityID, &ruleKey, &severity, &message, &email, &displayName, &systems) != nil {
			continue
		}
		if !highValueRuleKeys[ruleKey] {
			continue
		}
		var lastSent sql.NullTime
		_ = pool.QueryRow(ctx, `
			SELECT last_sent_at FROM alert_webhook_dedupe WHERE identity_id = $1 AND rule_key = $2
		`, identityID, ruleKey).Scan(&lastSent)
		if lastSent.Valid && time.Since(lastSent.Time) < dedupeWindow {
			continue
		}
		displayNameStr := ""
		if displayName != nil {
			displayNameStr = *displayName
		}
		payload := AlertPayload{
			Trigger:     ruleKey,
			IdentityID:  identityID,
			Email:       email,
			DisplayName: displayNameStr,
			Systems:     systems,
			DeepLink:    appBaseURL + "/identities/" + formatInt64(identityID),
			Severity:    severity,
			Message:     message,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			sent++
			_, _ = pool.Exec(ctx, `
				INSERT INTO alert_webhook_dedupe (identity_id, rule_key, last_sent_at)
				VALUES ($1, $2, NOW())
				ON CONFLICT (identity_id, rule_key) DO UPDATE SET last_sent_at = EXCLUDED.last_sent_at
			`, identityID, ruleKey)
		}
	}
	return sent, nil
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b) - 1
	for n > 0 {
		b[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	return string(b[i+1:])
}

// EmitAlerts POST /api/v1/alerts/emit - sends high-value triggers to ALERT_WEBHOOK_URL (last 1h)
func EmitAlerts(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		since := time.Now().Add(-1 * time.Hour)
		sent, err := NotifyHighValueTriggers(ctx, pool, since, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "sent": 0})
			return
		}
		c.JSON(http.StatusOK, gin.H{"sent": sent, "message": "Alerts emitted to ALERT_WEBHOOK_URL if configured"})
	}
}
