package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StitchingReviewItem is a pending human review for ambiguous identity correlation.
type StitchingReviewItem struct {
	ID         int64  `json:"id"`
	IdentityID int64  `json:"identity_id"`
	Email      string `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// ListStitchingReview returns pending queue items (populate via admin/SQL until automated detection ships).
func ListStitchingReview(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		rows, err := pool.Query(ctx, `
			SELECT q.id, q.identity_id, q.reason, q.status, q.created_at::text,
			       i.email, i.display_name
			FROM stitching_review_queue q
			JOIN identities i ON i.id = q.identity_id
			WHERE q.status = 'pending'
			ORDER BY q.created_at DESC
			LIMIT 100
		`)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "stitching_review_queue") && strings.Contains(msg, "does not exist") {
				msg = "Run migration 009 to enable the stitching review queue table."
			}
			c.JSON(http.StatusOK, gin.H{"items": []StitchingReviewItem{}, "count": 0, "message": msg})
			return
		}
		defer rows.Close()
		var items []StitchingReviewItem
		for rows.Next() {
			var it StitchingReviewItem
			if err := rows.Scan(&it.ID, &it.IdentityID, &it.Reason, &it.Status, &it.CreatedAt, &it.Email, &it.DisplayName); err != nil {
				continue
			}
			items = append(items, it)
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items)})
	}
}
