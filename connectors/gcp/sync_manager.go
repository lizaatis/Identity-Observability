package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncManager manages sync runs and snapshots
type SyncManager struct {
	pool *pgxpool.Pool
}

// NewSyncManager creates a new sync manager
func NewSyncManager(pool *pgxpool.Pool) *SyncManager {
	return &SyncManager{pool: pool}
}

// StartSyncRun starts a new sync run
func (sm *SyncManager) StartSyncRun(ctx context.Context, sourceSystem, connectorName string, metadata map[string]interface{}) (int64, error) {
	metadataJSON, _ := json.Marshal(metadata)

	var syncRunID int64
	err := sm.pool.QueryRow(ctx, `
		INSERT INTO sync_runs (source_system, connector_name, status, started_at, metadata)
		VALUES ($1, $2, 'running', NOW(), $3::jsonb)
		RETURNING id
	`, sourceSystem, connectorName, metadataJSON).Scan(&syncRunID)

	return syncRunID, err
}

// FinishSyncRun finishes a sync run
func (sm *SyncManager) FinishSyncRun(ctx context.Context, syncRunID int64, status string, errorCount, warningCount int, lastError *string) error {
	var errMsg interface{}
	if lastError != nil && *lastError != "" {
		errMsg = *lastError
	}

	_, err := sm.pool.Exec(ctx, `
		UPDATE sync_runs
		SET status = $1,
		    finished_at = NOW(),
		    error_count = $2,
		    warning_count = $3,
		    last_error = $4,
		    updated_at = NOW()
		WHERE id = $5
	`, status, errorCount, warningCount, errMsg, syncRunID)

	return err
}

// CreateEffectivePermissionSnapshot creates a snapshot of effective permissions
func (sm *SyncManager) CreateEffectivePermissionSnapshot(ctx context.Context, syncRunID int64) error {
	// Refresh the materialized view
	_, err := sm.pool.Exec(ctx, `
		REFRESH MATERIALIZED VIEW CONCURRENTLY identity_effective_permissions
	`)
	if err != nil {
		return fmt.Errorf("refresh effective permissions view: %w", err)
	}

	// Record snapshot metadata
	metadata := map[string]interface{}{
		"snapshot_at": time.Now().Format(time.RFC3339),
		"sync_run_id": syncRunID,
	}
	metadataJSON, _ := json.Marshal(metadata)

	_, err = sm.pool.Exec(ctx, `
		UPDATE sync_runs
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb
		WHERE id = $2
	`, metadataJSON, syncRunID)

	return err
}
