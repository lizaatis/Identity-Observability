package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncRun represents a connector sync run
type SyncRun struct {
	ID           int64
	SourceSystem string
	ConnectorName string
	StartedAt    time.Time
	FinishedAt   *time.Time
	Status       string // "running", "success", "error", "partial"
	ErrorCount   int
	WarningCount int
	LastError    *string
	Metadata     map[string]interface{}
}

// SyncManager handles sync run tracking and snapshot creation
type SyncManager struct {
	pool *pgxpool.Pool
}

// NewSyncManager creates a new sync manager
func NewSyncManager(pool *pgxpool.Pool) *SyncManager {
	return &SyncManager{pool: pool}
}

// StartSyncRun creates a new sync run record
func (sm *SyncManager) StartSyncRun(ctx context.Context, sourceSystem, connectorName string, metadata map[string]interface{}) (int64, error) {
	var syncRunID int64
	err := sm.pool.QueryRow(ctx, `
		INSERT INTO sync_runs (source_system, connector_name, status, started_at, metadata)
		VALUES ($1, $2, 'running', NOW(), $3)
		RETURNING id
	`, sourceSystem, connectorName, metadata).Scan(&syncRunID)
	return syncRunID, err
}

// FinishSyncRun updates a sync run with final status
func (sm *SyncManager) FinishSyncRun(ctx context.Context, syncRunID int64, status string, errorCount, warningCount int, lastError *string) error {
	_, err := sm.pool.Exec(ctx, `
		UPDATE sync_runs
		SET finished_at = NOW(),
		    status = $1,
		    error_count = $2,
		    warning_count = $3,
		    last_error = $4
		WHERE id = $5
	`, status, errorCount, warningCount, lastError, syncRunID)
	return err
}

// CreateEffectivePermissionSnapshot creates a snapshot of effective permissions for a sync run
func (sm *SyncManager) CreateEffectivePermissionSnapshot(ctx context.Context, syncRunID int64) error {
	// Delete any existing snapshots for this sync run (idempotent)
	_, err := sm.pool.Exec(ctx, `
		DELETE FROM effective_permission_snapshots WHERE sync_run_id = $1
	`, syncRunID)
	if err != nil {
		return err
	}

	// Insert snapshot from current effective_permissions view
	_, err = sm.pool.Exec(ctx, `
		INSERT INTO effective_permission_snapshots (
			sync_run_id, identity_id, permission_id, path_type, role_id, group_id, resource_id, created_at
		)
		SELECT 
			$1,
			identity_id,
			permission_id,
			path_type,
			role_id,
			group_id,
			NULL::BIGINT as resource_id,  -- Can be populated if resources are linked
			NOW()
		FROM identity_effective_permissions
	`, syncRunID)

	return err
}

// GetSyncRunStatus retrieves the status of a sync run
func (sm *SyncManager) GetSyncRunStatus(ctx context.Context, syncRunID int64) (*SyncRun, error) {
	var run SyncRun
	var finishedAt *time.Time
	var lastError *string
	var metadataJSON []byte

	err := sm.pool.QueryRow(ctx, `
		SELECT id, source_system, connector_name, started_at, finished_at, status,
		       error_count, warning_count, last_error, metadata
		FROM sync_runs
		WHERE id = $1
	`, syncRunID).Scan(
		&run.ID, &run.SourceSystem, &run.ConnectorName, &run.StartedAt,
		&finishedAt, &run.Status, &run.ErrorCount, &run.WarningCount,
		&lastError, &metadataJSON,
	)

	if err != nil {
		return nil, err
	}

	run.FinishedAt = finishedAt
	run.LastError = lastError
	// Parse metadata JSON if needed
	// For now, we'll leave it as nil since we're using JSONB

	return &run, nil
}

// GetLatestSyncRun gets the most recent sync run for a source system
func (sm *SyncManager) GetLatestSyncRun(ctx context.Context, sourceSystem string) (*SyncRun, error) {
	var run SyncRun
	var finishedAt *time.Time
	var lastError *string
	var metadataJSON []byte

	err := sm.pool.QueryRow(ctx, `
		SELECT id, source_system, connector_name, started_at, finished_at, status,
		       error_count, warning_count, last_error, metadata
		FROM sync_runs
		WHERE source_system = $1
		ORDER BY started_at DESC
		LIMIT 1
	`, sourceSystem).Scan(
		&run.ID, &run.SourceSystem, &run.ConnectorName, &run.StartedAt,
		&finishedAt, &run.Status, &run.ErrorCount, &run.WarningCount,
		&lastError, &metadataJSON,
	)

	if err != nil {
		return nil, err
	}

	run.FinishedAt = finishedAt
	run.LastError = lastError

	return &run, nil
}
