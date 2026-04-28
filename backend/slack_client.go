package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SlackClient handles Slack integration
type SlackClient struct {
	webhookURL string
	botToken   string
	channel    string
}

// NewSlackClient creates a new Slack client
func NewSlackClient() *SlackClient {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	channel := os.Getenv("SLACK_CHANNEL")
	if channel == "" {
		channel = "#identity-observability"
	}

	if webhookURL == "" && botToken == "" {
		return nil // Slack not configured
	}

	return &SlackClient{
		webhookURL: webhookURL,
		botToken:   botToken,
		channel:    channel,
	}
}

// SlackMessage represents a Slack message
type SlackMessage struct {
	Text        string                 `json:"text,omitempty"`
	Blocks      []SlackBlock           `json:"blocks,omitempty"`
	Channel     string                 `json:"channel,omitempty"`
	ResponseURL string                 `json:"response_url,omitempty"`
}

// SlackBlock represents a Slack block
type SlackBlock struct {
	Type    string                 `json:"type"`
	Text    *SlackText             `json:"text,omitempty"`
	Fields  []SlackText            `json:"fields,omitempty"`
	Elements []SlackElement        `json:"elements,omitempty"`
}

// SlackText represents text in Slack
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SlackElement represents an interactive element
type SlackElement struct {
	Type     string                 `json:"type"`
	Text     *SlackText             `json:"text,omitempty"`
	ActionID string                 `json:"action_id,omitempty"`
	Value    string                 `json:"value,omitempty"`
	Style    string                 `json:"style,omitempty"`
	URL      string                 `json:"url,omitempty"`
}

// PostApprovalRequest posts an approval request to Slack with approve/deny buttons
func (sc *SlackClient) PostApprovalRequest(ctx context.Context, actionID int64, identityID int64, req RemediationActionRequest) (string, error) {
	if sc == nil {
		return "", fmt.Errorf("slack client not configured")
	}

	message := SlackMessage{
		Channel: sc.channel,
		Blocks: []SlackBlock{
			{
				Type: "header",
				Text: &SlackText{
					Type: "plain_text",
					Text: "🔐 Remediation Action Approval Required",
				},
			},
			{
				Type: "section",
				Fields: []SlackText{
					{Type: "mrkdwn", Text: fmt.Sprintf("*Action:* %s", req.ActionType)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Target System:* %s", req.TargetSystem)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Identity ID:* %d", identityID)},
					{Type: "mrkdwn", Text: fmt.Sprintf("*Action ID:* %d", actionID)},
				},
			},
			{
				Type: "actions",
				Elements: []SlackElement{
					{
						Type:     "button",
						Text:     &SlackText{Type: "plain_text", Text: "✅ Approve"},
						ActionID: fmt.Sprintf("approve_%d", actionID),
						Value:    fmt.Sprintf("%d", actionID),
						Style:    "primary",
					},
					{
						Type:     "button",
						Text:     &SlackText{Type: "plain_text", Text: "❌ Deny"},
						ActionID: fmt.Sprintf("reject_%d", actionID),
						Value:    fmt.Sprintf("%d", actionID),
						Style:    "danger",
					},
				},
			},
		},
	}

	var messageID string
	var err error

	if sc.webhookURL != "" {
		// Use webhook
		messageID, err = sc.postWebhook(ctx, message)
	} else if sc.botToken != "" {
		// Use Bot API
		messageID, err = sc.postBotMessage(ctx, message)
	} else {
		return "", fmt.Errorf("no slack credentials configured")
	}

	return messageID, err
}

// postWebhook posts a message using webhook URL
func (sc *SlackClient) postWebhook(ctx context.Context, message SlackMessage) (string, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sc.webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Webhook doesn't return message ID, generate one
	return fmt.Sprintf("webhook_%d", time.Now().Unix()), nil
}

// postBotMessage posts a message using Bot API
func (sc *SlackClient) postBotMessage(ctx context.Context, message SlackMessage) (string, error) {
	// TODO: Implement Bot API posting
	// This would use chat.postMessage API
	return "", fmt.Errorf("bot API not yet implemented")
}

// HandleSlackInteraction handles Slack interactive message callbacks
func (sc *SlackClient) HandleSlackInteraction(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Slack sends form-encoded data
		var payload struct {
			Type    string `form:"type"`
			Actions []struct {
				ActionID string `json:"action_id"`
				Value    string `json:"value"`
			} `form:"actions"`
		}

		if err := c.ShouldBind(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		if payload.Type != "block_actions" {
			c.JSON(http.StatusOK, gin.H{"text": "Unknown interaction type"})
			return
		}

		for _, action := range payload.Actions {
			actionID, err := strconv.ParseInt(action.Value, 10, 64)
			if err != nil {
				continue
			}

			ctx := c.Request.Context()
			if strings.Contains(action.ActionID, "approve") {
				// Approve action
				_, err = pool.Exec(ctx, `
					UPDATE remediation_actions
					SET status = 'approved',
					    approved_by = 'slack',
					    approved_at = NOW()
					WHERE id = $1 AND status = 'pending'
				`, actionID)
			} else if strings.Contains(action.ActionID, "reject") {
				// Reject action
				_, err = pool.Exec(ctx, `
					UPDATE remediation_actions
					SET status = 'rejected'
					WHERE id = $1 AND status = 'pending'
				`, actionID)
			}

			if err == nil {
				c.JSON(http.StatusOK, gin.H{
					"response_type": "ephemeral",
					"text":          "Action processed",
				})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"text": "Processed"})
	}
}
