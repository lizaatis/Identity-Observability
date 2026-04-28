package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectorStatusResponse for GET /api/v1/connectors/:id/status
type ConnectorStatusResponse struct {
	ConnectorID    string                 `json:"connector_id"`
	SourceSystem   string                 `json:"source_system"`
	ConnectorName  string                 `json:"connector_name"`
	LastSync       *SyncRunStatusDTO      `json:"last_sync,omitempty"`
	RecentSyncs    []SyncRunStatusDTO     `json:"recent_syncs,omitempty"`
	Status         string                 `json:"status"` // "healthy", "degraded", "error"
}

type SyncRunStatusDTO struct {
	ID            int64     `json:"id"`
	StartedAt     string    `json:"started_at"`
	FinishedAt    *string   `json:"finished_at,omitempty"`
	Status        string    `json:"status"`
	ErrorCount    int       `json:"error_count"`
	WarningCount  int       `json:"warning_count"`
	LastError     *string   `json:"last_error,omitempty"`
	Duration      *string   `json:"duration,omitempty"`
}

// ListConnectorsResponse for GET /api/v1/connectors
type ListConnectorsResponse struct {
	Connectors []ConnectorListItem `json:"connectors"`
}

type ConnectorListItem struct {
	ConnectorID   string `json:"connector_id"`
	SourceSystem  string `json:"source_system"`
	ConnectorName string `json:"connector_name"`
	LastSyncAt    string `json:"last_sync_at,omitempty"`
	Status        string `json:"status"`
}

// ConnectorFullStatusItem is one connector with last sync details and optional row counts from sync metadata (single response, no N+1).
type ConnectorFullStatusItem struct {
	ConnectorListItem
	LastSync    *SyncRunStatusDTO      `json:"last_sync,omitempty"`
	RecentSyncs []SyncRunStatusDTO     `json:"recent_syncs,omitempty"`
	SyncMetadata map[string]interface{} `json:"sync_metadata,omitempty"`
	RowCounts   map[string]interface{} `json:"row_counts,omitempty"`
}

// List all connectors
func listConnectors(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// Get distinct connectors from sync_runs
		rows, err := pool.Query(ctx, `
			SELECT 
				COALESCE(connector_name, source_system) as connector_id,
				source_system,
				COALESCE(connector_name, source_system) as connector_name,
				MAX(started_at) as last_sync_at
			FROM sync_runs
			GROUP BY source_system, connector_name
			ORDER BY MAX(started_at) DESC
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var connectors []ConnectorListItem
		for rows.Next() {
			var conn ConnectorListItem
			var lastSyncAt *time.Time
			if err := rows.Scan(&conn.ConnectorID, &conn.SourceSystem, &conn.ConnectorName, &lastSyncAt); err != nil {
				continue
			}
			if lastSyncAt != nil {
				conn.LastSyncAt = lastSyncAt.Format(time.RFC3339)
			}
			conn.Status = "unknown" // Will be determined when fetching individual status
			connectors = append(connectors, conn)
		}

		// If no connectors found, return empty list
		c.JSON(http.StatusOK, ListConnectorsResponse{
			Connectors: connectors,
		})
	}
}

// listConnectorsFull returns per-connector health in one round-trip (last sync, duration, errors, metadata row counts).
func listConnectorsFull(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		rows, err := pool.Query(ctx, `
			SELECT DISTINCT ON (source_system, COALESCE(connector_name, source_system))
				id, source_system, COALESCE(connector_name, source_system), started_at, finished_at,
				status, error_count, warning_count, last_error, metadata
			FROM sync_runs
			ORDER BY source_system, COALESCE(connector_name, source_system), started_at DESC NULLS LAST
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var out []ConnectorFullStatusItem
		for rows.Next() {
			var syncID int64
			var sourceSystem, connectorName string
			var startedAt time.Time
			var finishedAt *time.Time
			var status string
			var errCount, warnCount int
			var lastErr *string
			var metaBytes []byte
			if err := rows.Scan(&syncID, &sourceSystem, &connectorName, &startedAt, &finishedAt,
				&status, &errCount, &warnCount, &lastErr, &metaBytes); err != nil {
				continue
			}
			last := buildSyncRunDTO(startedAt, finishedAt, syncID, status, errCount, warnCount, lastErr)
			meta := mapMetadata(metaBytes)
			item := ConnectorFullStatusItem{
				ConnectorListItem: ConnectorListItem{
					ConnectorID:   connectorName,
					SourceSystem:  sourceSystem,
					ConnectorName: connectorName,
					LastSyncAt:    startedAt.Format(time.RFC3339),
					Status:        overallConnectorStatus(status, errCount),
				},
				LastSync:     &last,
				SyncMetadata: meta,
				RowCounts:    extractRowCountsFromMetadata(meta),
			}
			recent, _ := fetchRecentSyncsForConnector(ctx, pool, sourceSystem, connectorName)
			item.RecentSyncs = recent
			out = append(out, item)
		}
		c.JSON(http.StatusOK, gin.H{"connectors": out})
	}
}

func mapMetadata(metaBytes []byte) map[string]interface{} {
	if len(metaBytes) == 0 {
		return nil
	}
	var m map[string]interface{}
	if json.Unmarshal(metaBytes, &m) != nil {
		return nil
	}
	return m
}

func extractRowCountsFromMetadata(meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		return nil
	}
	keys := []string{
		"identities_upserted", "identities_synced", "rows_processed", "rows_upserted",
		"users_synced", "groups_synced", "row_count",
	}
	out := make(map[string]interface{})
	for _, k := range keys {
		if v, ok := meta[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func overallConnectorStatus(status string, errCount int) string {
	if status == "error" {
		return "error"
	}
	if status == "partial" || errCount > 0 {
		return "degraded"
	}
	if status == "success" || status == "running" {
		return "healthy"
	}
	return "unknown"
}

func buildSyncRunDTO(startedAt time.Time, finishedAt *time.Time, id int64, status string, errCount, warnCount int, lastErr *string) SyncRunStatusDTO {
	dto := SyncRunStatusDTO{
		ID:           id,
		StartedAt:    startedAt.Format(time.RFC3339),
		Status:       status,
		ErrorCount:   errCount,
		WarningCount: warnCount,
		LastError:    lastErr,
	}
	if finishedAt != nil {
		fs := finishedAt.Format(time.RFC3339)
		dto.FinishedAt = &fs
		dur := finishedAt.Sub(startedAt).String()
		dto.Duration = &dur
	}
	return dto
}

func fetchRecentSyncsForConnector(ctx context.Context, pool *pgxpool.Pool, sourceSystem, connectorName string) ([]SyncRunStatusDTO, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, started_at, finished_at, status, error_count, warning_count, last_error
		FROM sync_runs
		WHERE source_system = $1 AND COALESCE(connector_name, source_system) = $2
		ORDER BY started_at DESC
		LIMIT 10
	`, sourceSystem, connectorName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recent []SyncRunStatusDTO
	for rows.Next() {
		var sync SyncRunStatusDTO
		var started, finished *time.Time
		var lastE *string
		if err := rows.Scan(&sync.ID, &started, &finished, &sync.Status,
			&sync.ErrorCount, &sync.WarningCount, &lastE); err != nil {
			continue
		}
		if started != nil {
			sync.StartedAt = started.Format(time.RFC3339)
		}
		if finished != nil {
			fs := finished.Format(time.RFC3339)
			sync.FinishedAt = &fs
			if started != nil {
				dur := finished.Sub(*started).String()
				sync.Duration = &dur
			}
		}
		sync.LastError = lastE
		recent = append(recent, sync)
	}
	return recent, nil
}

// Get connector status
func getConnectorStatus(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		connectorID := c.Param("id")
		ctx := c.Request.Context()

		// Get latest sync run for this connector
		var lastSync SyncRunStatusDTO
		var startedAt, finishedAt time.Time
		var finishedAtPtr *time.Time
		var lastError *string
		var duration *string

		err := pool.QueryRow(ctx, `
			SELECT id, started_at, finished_at, status, error_count, warning_count, last_error
			FROM sync_runs
			WHERE source_system = $1 OR connector_name = $1
			ORDER BY started_at DESC
			LIMIT 1
		`, connectorID).Scan(
			&lastSync.ID, &startedAt, &finishedAtPtr, &lastSync.Status,
			&lastSync.ErrorCount, &lastSync.WarningCount, &lastError,
		)

		if err == nil {
			lastSync.StartedAt = startedAt.Format(time.RFC3339)
			if finishedAtPtr != nil {
				finishedAt = *finishedAtPtr
				finishedAtStr := finishedAt.Format(time.RFC3339)
				lastSync.FinishedAt = &finishedAtStr
				
				dur := finishedAt.Sub(startedAt).String()
				duration = &dur
				lastSync.Duration = duration
			}
			lastSync.LastError = lastError
		}

		// Get recent syncs (last 10)
		rows, err := pool.Query(ctx, `
			SELECT id, started_at, finished_at, status, error_count, warning_count, last_error
			FROM sync_runs
			WHERE source_system = $1 OR connector_name = $1
			ORDER BY started_at DESC
			LIMIT 10
		`, connectorID)

		var recentSyncs []SyncRunStatusDTO
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sync SyncRunStatusDTO
				var started, finished *time.Time
				var lastErr *string
				
				if err := rows.Scan(&sync.ID, &started, &finished, &sync.Status,
					&sync.ErrorCount, &sync.WarningCount, &lastErr); err != nil {
					continue
				}

				if started != nil {
					sync.StartedAt = started.Format(time.RFC3339)
				}
				if finished != nil {
					finishedStr := finished.Format(time.RFC3339)
					sync.FinishedAt = &finishedStr
					if started != nil {
						dur := finished.Sub(*started).String()
						sync.Duration = &dur
					}
				}
				sync.LastError = lastErr
				recentSyncs = append(recentSyncs, sync)
			}
		}

		// Determine overall status
		overallStatus := "healthy"
		if len(recentSyncs) > 0 {
			latest := recentSyncs[0]
			if latest.Status == "error" {
				overallStatus = "error"
			} else if latest.Status == "partial" || latest.ErrorCount > 0 {
				overallStatus = "degraded"
			}
		}

		// Get source system and connector name from latest sync
		var sourceSystem, connectorName string
		_ = pool.QueryRow(ctx, `
			SELECT source_system, connector_name
			FROM sync_runs
			WHERE source_system = $1 OR connector_name = $1
			ORDER BY started_at DESC
			LIMIT 1
		`, connectorID).Scan(&sourceSystem, &connectorName)

		c.JSON(http.StatusOK, ConnectorStatusResponse{
			ConnectorID:   connectorID,
			SourceSystem:  sourceSystem,
			ConnectorName: connectorName,
			LastSync:      &lastSync,
			RecentSyncs:   recentSyncs,
			Status:        overallStatus,
		})
	}
}
