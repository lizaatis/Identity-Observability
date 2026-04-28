package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RemediationActionRequest represents a remediation action request
type RemediationActionRequest struct {
	ActionType   string                 `json:"action_type" binding:"required"` // 'disable_user', 'remove_permission', 'remove_group', 'create_ticket'
	TargetSystem string                 `json:"target_system" binding:"required"`
	TargetID     string                 `json:"target_id,omitempty"`
	RequiresApproval bool               `json:"requires_approval"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// RemediationActionResponse represents a remediation action
type RemediationActionResponse struct {
	ID            int64                  `json:"id"`
	IdentityID    int64                  `json:"identity_id"`
	ActionType    string                 `json:"action_type"`
	TargetSystem  string                 `json:"target_system"`
	Status        string                 `json:"status"`
	RequestedBy   string                 `json:"requested_by"`
	RequestedAt   string                 `json:"requested_at"`
	ApprovedBy    *string                `json:"approved_by,omitempty"`
	ApprovedAt    *string               `json:"approved_at,omitempty"`
	ExecutedAt    *string               `json:"executed_at,omitempty"`
	ErrorMessage  *string                `json:"error_message,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CreateRemediationAction creates a new remediation action
func CreateRemediationAction(pool *pgxpool.Pool, slackClient *SlackClient, jiraClient *JiraClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		identityID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || identityID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid identity id"})
			return
		}

		var req RemediationActionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}

		if c.Query("dry_run") == "true" || c.Query("simulate") == "true" {
			c.JSON(http.StatusOK, gin.H{
				"dry_run":       true,
				"identity_id":   identityID,
				"would_create":  req,
				"message":       "No rows written. Omit dry_run/simulate to create the remediation action.",
			})
			return
		}

		// Get requesting user (from auth context, default to "system" for now)
		requestedBy := c.GetString("user_email")
		if requestedBy == "" {
			requestedBy = "system"
		}

		ctx := c.Request.Context()

		// Create remediation action
		var actionID int64
		err = pool.QueryRow(ctx, `
			INSERT INTO remediation_actions (
				identity_id, action_type, target_system, target_id,
				status, requested_by, metadata
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, identityID, req.ActionType, req.TargetSystem, req.TargetID,
			"pending", requestedBy, req.Metadata).Scan(&actionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create action", "details": err.Error()})
			return
		}

		// If requires approval, create approval request
		if req.RequiresApproval {
			// Determine approval channel
			channel := "slack" // Default to Slack
			if slackClient == nil {
				channel = "internal"
			}

			var approvalID int64
			if channel == "slack" && slackClient != nil {
				// Post to Slack with approve/deny buttons
				messageID, err := slackClient.PostApprovalRequest(ctx, actionID, identityID, req)
				if err != nil {
					// Fallback to internal approval
					channel = "internal"
					messageID = ""
				}

				// Create approval request
				err = pool.QueryRow(ctx, `
					INSERT INTO approval_requests (
						remediation_action_id, channel, channel_id, message_id, expires_at
					) VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours')
					RETURNING id
				`, actionID, channel, "", messageID).Scan(&approvalID)
				if err != nil {
					// Log error but continue
				}
			} else {
				// Internal approval (manual)
				err = pool.QueryRow(ctx, `
					INSERT INTO approval_requests (
						remediation_action_id, channel, expires_at
					) VALUES ($1, $2, NOW() + INTERVAL '24 hours')
					RETURNING id
				`, actionID, channel).Scan(&approvalID)
				if err != nil {
					// Log error but continue
				}
			}
		} else {
			// Execute immediately if no approval required
			go executeRemediationAction(context.Background(), pool, actionID, req)
		}

		// Return created action
		var action RemediationActionResponse
		err = pool.QueryRow(ctx, `
			SELECT id, identity_id, action_type, target_system, status,
			       requested_by, requested_at::text, approved_by, approved_at::text,
			       executed_at::text, error_message, metadata
			FROM remediation_actions
			WHERE id = $1
		`, actionID).Scan(
			&action.ID, &action.IdentityID, &action.ActionType, &action.TargetSystem,
			&action.Status, &action.RequestedBy, &action.RequestedAt,
			&action.ApprovedBy, &action.ApprovedAt, &action.ExecutedAt,
			&action.ErrorMessage, &action.Metadata,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch action"})
			return
		}

		c.JSON(http.StatusCreated, action)
	}
}

// ApproveRemediationAction approves a remediation action
func ApproveRemediationAction(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		actionID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || actionID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action id"})
			return
		}

		approvedBy := c.GetString("user_email")
		if approvedBy == "" {
			approvedBy = "system"
		}

		ctx := c.Request.Context()

		// Update action status
		_, err = pool.Exec(ctx, `
			UPDATE remediation_actions
			SET status = 'approved',
			    approved_by = $1,
			    approved_at = NOW()
			WHERE id = $2 AND status = 'pending'
		`, approvedBy, actionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve action"})
			return
		}

		// Update approval request
		_, err = pool.Exec(ctx, `
			UPDATE approval_requests
			SET status = 'approved',
			    updated_at = NOW()
			WHERE remediation_action_id = $1 AND status = 'pending'
		`, actionID)
		if err != nil {
			// Log error but continue
		}

		// Get action details and execute
		var req RemediationActionRequest
		var targetSystem, actionType string
		var metadataJSON []byte
		err = pool.QueryRow(ctx, `
			SELECT action_type, target_system, metadata
			FROM remediation_actions
			WHERE id = $1
		`, actionID).Scan(&actionType, &targetSystem, &metadataJSON)
		if err == nil {
			json.Unmarshal(metadataJSON, &req.Metadata)
			req.ActionType = actionType
			req.TargetSystem = targetSystem
			go executeRemediationAction(context.Background(), pool, actionID, req)
		}

		c.JSON(http.StatusOK, gin.H{"status": "approved", "action_id": actionID})
	}
}

// RejectRemediationAction rejects a remediation action
func RejectRemediationAction(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		actionID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || actionID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action id"})
			return
		}

		ctx := c.Request.Context()

		// Update action status
		_, err = pool.Exec(ctx, `
			UPDATE remediation_actions
			SET status = 'rejected'
			WHERE id = $1 AND status = 'pending'
		`, actionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject action"})
			return
		}

		// Update approval request
		_, err = pool.Exec(ctx, `
			UPDATE approval_requests
			SET status = 'rejected',
			    updated_at = NOW()
			WHERE remediation_action_id = $1 AND status = 'pending'
		`, actionID)
		if err != nil {
			// Log error but continue
		}

		c.JSON(http.StatusOK, gin.H{"status": "rejected", "action_id": actionID})
	}
}

// executeRemediationAction executes a remediation action
func executeRemediationAction(ctx context.Context, pool *pgxpool.Pool, actionID int64, req RemediationActionRequest) {
	// Update status to executing
	pool.Exec(ctx, `
		UPDATE remediation_actions
		SET status = 'executing'
		WHERE id = $1
	`, actionID)

	var err error
	var errorMsg string

	switch req.ActionType {
	case "disable_user":
		err = executeDisableUser(ctx, pool, actionID, req)
	case "remove_permission":
		err = executeRemovePermission(ctx, pool, actionID, req)
	case "remove_group":
		err = executeRemoveGroup(ctx, pool, actionID, req)
	case "create_ticket":
		err = executeCreateTicket(ctx, pool, actionID, req)
	default:
		err = fmt.Errorf("unknown action type: %s", req.ActionType)
	}

	// Update action status
	status := "executed"
	if err != nil {
		status = "failed"
		errorMsg = err.Error()
	}

	pool.Exec(ctx, `
		UPDATE remediation_actions
		SET status = $1,
		    executed_at = NOW(),
		    error_message = $2
		WHERE id = $3
	`, status, errorMsg, actionID)

	// If successful, create remediation history entry
	if err == nil {
		var identityID int64
		var riskType string
		pool.QueryRow(ctx, `
			SELECT identity_id, 
			       COALESCE((SELECT rule_key FROM risk_flags WHERE id = (SELECT risk_flag_id FROM remediation_actions WHERE id = $1)), 'unknown')
			FROM remediation_actions
			WHERE id = $1
		`, actionID).Scan(&identityID, &riskType)

		pool.Exec(ctx, `
			INSERT INTO remediation_history (
				remediation_action_id, identity_id, risk_type, action_type, status
			) VALUES ($1, $2, $3, $4, 'executed')
		`, actionID, identityID, riskType, req.ActionType)
	}
}

// Placeholder execution functions (to be implemented with actual API calls)
func executeDisableUser(ctx context.Context, pool *pgxpool.Pool, actionID int64, req RemediationActionRequest) error {
	// TODO: Call target system API to disable user
	// For now, just log
	return nil
}

func executeRemovePermission(ctx context.Context, pool *pgxpool.Pool, actionID int64, req RemediationActionRequest) error {
	// TODO: Call target system API to remove permission
	return nil
}

func executeRemoveGroup(ctx context.Context, pool *pgxpool.Pool, actionID int64, req RemediationActionRequest) error {
	// TODO: Call target system API to remove group membership
	return nil
}

func executeCreateTicket(ctx context.Context, pool *pgxpool.Pool, actionID int64, req RemediationActionRequest) error {
	// TODO: Create Jira ticket
	return nil
}
