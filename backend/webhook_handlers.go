package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OktaWebhookEvent represents an Okta System Log event
type OktaWebhookEvent struct {
	EventType     string                 `json:"eventType"`
	DisplayMessage string                `json:"displayMessage"`
	Severity      string                 `json:"severity"`
	Outcome       string                 `json:"outcome"`
	Published     string                 `json:"published"` // ISO 8601 timestamp
	Target        []OktaEventTarget      `json:"target"`
	Actor         OktaEventActor         `json:"actor"`
	Client        OktaEventClient        `json:"client"`
	Request       OktaEventRequest      `json:"request"`
	DebugContext  map[string]interface{} `json:"debugContext"`
	AuthenticationContext map[string]interface{} `json:"authenticationContext"`
}

type OktaEventTarget struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	AlternateID string `json:"alternateId"`
	DisplayName string `json:"displayName"`
}

type OktaEventActor struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	AlternateID string `json:"alternateId"`
	DisplayName string `json:"displayName"`
}

type OktaEventClient struct {
	UserAgent struct {
		RawUserAgent string `json:"rawUserAgent"`
		OS           string `json:"os"`
		Browser      string `json:"browser"`
	} `json:"userAgent"`
	IPAddress string `json:"ipAddress"`
}

type OktaEventRequest struct {
	IPChain []struct {
		IP   string `json:"ip"`
		Geolocation struct {
			Country string `json:"country"`
			Region  string `json:"region"`
		} `json:"geolocation"`
	} `json:"ipChain"`
}

// WebhookHandler handles incoming webhooks from IdP systems
type WebhookHandler struct {
	pool       *pgxpool.Pool
	eventQueue EventQueue
}

// QueuedEvent represents an event queued for processing
type QueuedEvent struct {
	SourceSystem string
	EventType    string
	SourceUserID string
	EventData    map[string]interface{}
	EventTime    time.Time
	StreamID     string // For Redis Streams ACK
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(pool *pgxpool.Pool, eventQueue EventQueue) *WebhookHandler {
	return &WebhookHandler{
		pool:       pool,
		eventQueue: eventQueue,
	}
}

// HandleOktaWebhook processes Okta System Log webhook events
func (wh *WebhookHandler) HandleOktaWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		var events []OktaWebhookEvent
		
		// Okta can send single event or array
		if err := c.ShouldBindJSON(&events); err != nil {
			// Try single event
			var singleEvent OktaWebhookEvent
			if err2 := c.ShouldBindJSON(&singleEvent); err2 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON", "details": err.Error()})
				return
			}
			events = []OktaWebhookEvent{singleEvent}
		}

		// Queue events for async processing
		for _, event := range events {
			// Extract user ID from target (Okta user ID)
			sourceUserID := ""
			for _, target := range event.Target {
				if target.Type == "User" {
					sourceUserID = target.ID
					break
				}
			}

			// Parse event time
			eventTime := time.Now()
			if event.Published != "" {
				if parsed, err := time.Parse(time.RFC3339, event.Published); err == nil {
					eventTime = parsed
				}
			}

			// Convert to map for JSONB storage
			eventData, _ := json.Marshal(event)
			var eventDataMap map[string]interface{}
			json.Unmarshal(eventData, &eventDataMap)

			queuedEvent := &QueuedEvent{
				SourceSystem: "okta_prod",
				EventType:    event.EventType,
				SourceUserID: sourceUserID,
				EventData:    eventDataMap,
				EventTime:    eventTime,
			}

			// Enqueue event (uses Redis Streams if configured, otherwise in-memory)
			if err := wh.eventQueue.Enqueue(c.Request.Context(), queuedEvent); err != nil {
				// Log error but don't fail the webhook (events are still accepted)
				// In production, consider adding structured logging
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "accepted",
			"count":   len(events),
			"message": "events queued for processing",
		})
	}
}

// StartEventProcessor starts the background event processor
func (wh *WebhookHandler) StartEventProcessor(ctx context.Context, consumerGroup string, consumerName string) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Dequeue events (batch of 10)
				events, err := wh.eventQueue.Dequeue(ctx, consumerGroup, consumerName, 10)
				if err != nil {
					// Log error and continue
					continue
				}

				// Process each event
				for _, event := range events {
					if err := wh.processEvent(ctx, event); err == nil && event.StreamID != "" {
						// ACK successful processing (for Redis Streams)
						wh.eventQueue.Ack(ctx, consumerGroup, event.StreamID)
					}
				}

				// Small delay to avoid tight loop
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// processEvent processes a single event: stores it and resolves identity
func (wh *WebhookHandler) processEvent(ctx context.Context, event *QueuedEvent) error {
	// 1. Resolve identity_id from source_user_id
	var identityID *int64
	if event.SourceUserID != "" {
		err := wh.pool.QueryRow(ctx, `
			SELECT identity_id 
			FROM identity_sources 
			WHERE source_system = $1 AND source_user_id = $2
			LIMIT 1
		`, event.SourceSystem, event.SourceUserID).Scan(&identityID)
		if err != nil {
			// Identity not yet resolved, will be resolved on next sync
			identityID = nil
		}
	}

	// 2. Store event in identity_events table
	eventDataJSON, _ := json.Marshal(event.EventData)
	_, err := wh.pool.Exec(ctx, `
		INSERT INTO identity_events (
			event_time, source_system, event_type, 
			identity_id, source_user_id, event_data, processed
		) VALUES ($1, $2, $3, $4, $5, $6, TRUE)
	`, event.EventTime, event.SourceSystem, event.EventType, 
		identityID, event.SourceUserID, eventDataJSON)
	
	if err != nil {
		return fmt.Errorf("store event: %w", err)
	}

	// 3. If identity was resolved, trigger risk recalculation (async)
	if identityID != nil {
		// Optionally trigger risk score recalculation
		// This could be done via a separate worker or trigger
	}

	return nil
}
