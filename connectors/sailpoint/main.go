package sailpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SailPointConnector syncs data from SailPoint IdentityNow to canonical schema
type SailPointConnector struct {
	config      *Config
	pool        *pgxpool.Pool
	client      *Client
	resolution  *IdentityResolution
	rateLimiter *RateLimiter
	syncManager *SyncManager
	errorCount  int
	warningCount int
	lastError   string
}

// NewSailPointConnector creates a new SailPoint connector
func NewSailPointConnector(cfg *Config) (*SailPointConnector, error) {
	// Validate config
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Tenant == "" {
		return nil, fmt.Errorf("SAILPOINT_TENANT is required")
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("SAILPOINT_CLIENT_ID is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("SAILPOINT_CLIENT_SECRET is required")
	}

	// Connect to database
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database connection: %w", err)
	}

	// Create SailPoint client
	auth := NewAuth(cfg.Tenant, cfg.BaseURL, cfg.ClientID, cfg.ClientSecret)
	client := NewClient(cfg.Tenant, cfg.BaseURL, auth)

	// Create sync manager
	syncMgr := NewSyncManager(pool)

	// Load privilege markers (Component 5: Privilege Signals)
	if err := LoadPrivilegeMarkers(""); err != nil {
		log.Printf("Warning: Could not load privilege markers: %v", err)
	}

	return &SailPointConnector{
		config:      cfg,
		pool:        pool,
		client:      client,
		resolution:  &IdentityResolution{pool: pool, minConfidence: cfg.MinConfidenceScore},
		rateLimiter: NewRateLimiter(cfg.MaxRetries, cfg.RetryBackoff, cfg.RateLimitWindow),
		syncManager: syncMgr,
	}, nil
}

// Sync runs a full sync from SailPoint
func (sc *SailPointConnector) Sync(ctx context.Context) error {
	log.Println("Starting SailPoint sync...")

	// Start sync run
	syncRunID, err := sc.syncManager.StartSyncRun(ctx, sc.config.SourceSystem, sc.config.ConnectorName, map[string]interface{}{
		"incremental": sc.config.IncrementalSync,
		"changed_since": sc.config.ChangedSince,
	})
	if err != nil {
		return fmt.Errorf("start sync run: %w", err)
	}

	defer func() {
		status := "success"
		if sc.errorCount > 0 {
			status = "partial"
		}
		if err != nil {
			status = "error"
		}
		sc.syncManager.FinishSyncRun(ctx, syncRunID, status, sc.errorCount, sc.warningCount, &sc.lastError)
	}()

	// Sync identities
	if err := sc.syncIdentities(ctx); err != nil {
		sc.recordError(fmt.Sprintf("sync identities: %v", err))
		return err
	}

	// Sync access profiles (as roles)
	if err := sc.syncAccessProfiles(ctx); err != nil {
		sc.recordError(fmt.Sprintf("sync access profiles: %v", err))
		return err
	}

	// Sync roles
	if err := sc.syncRoles(ctx); err != nil {
		sc.recordError(fmt.Sprintf("sync roles: %v", err))
		return err
	}

	// Sync entitlements (as permissions)
	if err := sc.syncEntitlements(ctx); err != nil {
		sc.recordError(fmt.Sprintf("sync entitlements: %v", err))
		return err
	}

	// Component 3: Account Objects - Sync Linked Accounts
	if err := sc.syncLinkedAccounts(ctx); err != nil {
		sc.recordWarning(fmt.Sprintf("sync linked accounts: %v", err))
		// Don't fail sync if linked accounts fail
	}

	// Sync identity access (role assignments)
	if err := sc.syncIdentityAccess(ctx); err != nil {
		sc.recordError(fmt.Sprintf("sync identity access: %v", err))
		return err
	}

	// Component 6: Governance Signals
	if err := sc.syncGovernanceSignals(ctx); err != nil {
		sc.recordWarning(fmt.Sprintf("sync governance signals: %v", err))
		// Governance is optional
	}

	// Create effective permission snapshot
	if err := sc.syncManager.CreateEffectivePermissionSnapshot(ctx, syncRunID); err != nil {
		sc.recordWarning(fmt.Sprintf("create snapshot: %v", err))
	}

	log.Printf("Sync complete. Errors: %d, Warnings: %d", sc.errorCount, sc.warningCount)

	// Report low-confidence matches
	lowConf := sc.resolution.GetLowConfidenceMatches()
	if len(lowConf) > 0 {
		log.Printf("⚠️  %d low-confidence identity matches need manual review", len(lowConf))
		for _, match := range lowConf {
			log.Printf("  - SailPoint identity %s matched to identity %d (confidence: %.2f, method: %s)",
				match.SailPointUserID, match.CanonicalID, match.Confidence, match.MatchMethod)
		}
	}

	return nil
}

// syncIdentities syncs SailPoint identities to canonical identities
func (sc *SailPointConnector) syncIdentities(ctx context.Context) error {
	log.Println("Syncing identities...")

	limit := 250
	offset := 0
	totalSynced := 0

	for {
		sc.rateLimiter.WaitIfNeeded(ctx)

		var resp *ListIdentitiesResponse
		err := sc.rateLimiter.RetryWithBackoff(ctx, func() error {
			var err error
			resp, err = sc.client.ListIdentities(ctx, limit, offset)
			return err
		})

		if err != nil {
			return fmt.Errorf("list identities: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		for _, identity := range resp.Items {
			if err := sc.upsertIdentity(ctx, identity); err != nil {
				sc.recordError(fmt.Sprintf("upsert identity %s: %v", identity.ID, err))
				continue
			}
			totalSynced++
		}

		// Check if there are more pages
		if offset+len(resp.Items) >= resp.Total {
			break
		}
		offset += limit
	}

	log.Printf("Synced %d identities", totalSynced)
	return nil
}

// upsertIdentity upserts a single SailPoint identity
func (sc *SailPointConnector) upsertIdentity(ctx context.Context, identity Identity) error {
	// Extract fields
	identityID := identity.ID
	email := identity.Email
	employeeID := identity.EmployeeID
	displayName := ""
	status := "active"

	// Extract display name - can be string or map
	if identity.Name != nil {
		switch v := identity.Name.(type) {
		case string:
			displayName = v
		case map[string]interface{}:
			if first, ok := v["first"].(string); ok {
				displayName = first
				if last, ok := v["last"].(string); ok {
					displayName = first + " " + last
				}
			}
		}
	}

	// Normalize status
	if identity.Status == "INACTIVE" || identity.Status == "DELETED" {
		status = "inactive"
	}

	// Skip identities without email (they can't be matched)
	if email == "" {
		sc.recordWarning(fmt.Sprintf("identity %s has no email, skipping", identityID))
		return nil
	}

	// Resolve identity
	canonicalID, confidence, _, err := sc.resolution.ResolveIdentity(ctx, identityID, email, employeeID)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Create new identity if not found
	if canonicalID == 0 {
		canonicalID, err = sc.resolution.CreateOrGetIdentity(ctx, email, displayName, employeeID, status)
		if err != nil {
			return fmt.Errorf("create identity: %w", err)
		}
	}

	// Upsert identity_sources
	_, err = sc.pool.Exec(ctx, `
		INSERT INTO identity_sources (identity_id, source_system, source_user_id, source_status, confidence, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_user_id) DO UPDATE SET
			identity_id = $1,
			source_status = $4,
			confidence = $5,
			synced_at = NOW(),
			updated_at = NOW()
	`, canonicalID, sc.config.SourceSystem, identityID, status, confidence)

	return err
}

// syncAccessProfiles syncs SailPoint access profiles as roles
func (sc *SailPointConnector) syncAccessProfiles(ctx context.Context) error {
	log.Println("Syncing access profiles (as roles)...")

	limit := 250
	offset := 0

	for {
		sc.rateLimiter.WaitIfNeeded(ctx)

		var resp *ListAccessProfilesResponse
		err := sc.rateLimiter.RetryWithBackoff(ctx, func() error {
			var err error
			resp, err = sc.client.ListAccessProfiles(ctx, limit, offset)
			return err
		})

		if err != nil {
			return fmt.Errorf("list access profiles: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		for _, profile := range resp.Items {
			if err := sc.upsertAccessProfile(ctx, profile); err != nil {
				sc.recordError(fmt.Sprintf("upsert access profile %s: %v", profile.ID, err))
				continue
			}
		}

		if offset+len(resp.Items) >= resp.Total {
			break
		}
		offset += limit
	}

	return nil
}

// upsertAccessProfile upserts an access profile as a role
// Component 5: Privilege Signals - Uses privilege markers to determine privilege level
func (sc *SailPointConnector) upsertAccessProfile(ctx context.Context, profile AccessProfile) error {
	// Determine privilege level using privilege markers
	privilegeLevel := string(GetPrivilegeLevel(profile.Name))
	if privilegeLevel == "standard" {
		privilegeLevel = "read" // Default for access profiles
	}

	_, err := sc.pool.Exec(ctx, `
		INSERT INTO roles (name, privilege_level, source_system, source_id, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_id) DO UPDATE SET
			name = $1,
			privilege_level = $2,
			synced_at = NOW(),
			updated_at = NOW()
	`, profile.Name, privilegeLevel, sc.config.SourceSystem, "accessprofile_"+profile.ID)

	return err
}

// syncRoles syncs SailPoint roles
func (sc *SailPointConnector) syncRoles(ctx context.Context) error {
	log.Println("Syncing roles...")

	limit := 250
	offset := 0

	for {
		sc.rateLimiter.WaitIfNeeded(ctx)

		var resp *ListRolesResponse
		err := sc.rateLimiter.RetryWithBackoff(ctx, func() error {
			var err error
			resp, err = sc.client.ListRoles(ctx, limit, offset)
			return err
		})

		if err != nil {
			return fmt.Errorf("list roles: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		for _, role := range resp.Items {
			if err := sc.upsertRole(ctx, role); err != nil {
				sc.recordError(fmt.Sprintf("upsert role %s: %v", role.ID, err))
				continue
			}
		}

		if offset+len(resp.Items) >= resp.Total {
			break
		}
		offset += limit
	}

	return nil
}

// upsertRole upserts a SailPoint role
// Component 5: Privilege Signals - Uses privilege markers to determine privilege level
func (sc *SailPointConnector) upsertRole(ctx context.Context, role Role) error {
	// Determine privilege level using privilege markers
	privilegeLevel := string(GetPrivilegeLevel(role.Name))
	if privilegeLevel == "standard" {
		privilegeLevel = "read" // Default for roles
	}

	_, err := sc.pool.Exec(ctx, `
		INSERT INTO roles (name, privilege_level, source_system, source_id, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_id) DO UPDATE SET
			name = $1,
			privilege_level = $2,
			synced_at = NOW(),
			updated_at = NOW()
	`, role.Name, privilegeLevel, sc.config.SourceSystem, "role_"+role.ID)

	return err
}

// syncEntitlements syncs SailPoint entitlements as permissions
func (sc *SailPointConnector) syncEntitlements(ctx context.Context) error {
	log.Println("Syncing entitlements (as permissions)...")

	limit := 250
	offset := 0

	for {
		sc.rateLimiter.WaitIfNeeded(ctx)

		var resp *ListEntitlementsResponse
		err := sc.rateLimiter.RetryWithBackoff(ctx, func() error {
			var err error
			resp, err = sc.client.ListEntitlements(ctx, limit, offset)
			if err != nil && contains(err.Error(), "endpoint_not_found") {
				// Don't retry 404 errors
				return err
			}
			return err
		})

		if err != nil {
			// Check if it's a 404 - entitlements endpoint may not be available
			errStr := err.Error()
			if strings.Contains(errStr, "endpoint_not_found") || strings.Contains(errStr, "404") {
				sc.recordWarning("Entitlements endpoint not available (404) - skipping entitlements sync")
				log.Println("Skipping entitlements sync - endpoint not available")
				return nil // Continue with rest of sync
			}
			return fmt.Errorf("list entitlements: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		for _, entitlement := range resp.Items {
			if err := sc.upsertEntitlement(ctx, entitlement); err != nil {
				sc.recordError(fmt.Sprintf("upsert entitlement %s: %v", entitlement.ID, err))
				continue
			}
		}

		if offset+len(resp.Items) >= resp.Total {
			break
		}
		offset += limit
	}

	return nil
}

// upsertEntitlement upserts an entitlement as a permission
func (sc *SailPointConnector) upsertEntitlement(ctx context.Context, entitlement Entitlement) error {
	_, err := sc.pool.Exec(ctx, `
		INSERT INTO permissions (name, permission_type, source_system, source_id, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_id) DO UPDATE SET
			name = $1,
			permission_type = $2,
			synced_at = NOW(),
			updated_at = NOW()
	`, entitlement.Name, entitlement.Type, sc.config.SourceSystem, "entitlement_"+entitlement.ID)

	return err
}

// syncIdentityAccess syncs identity access (role assignments)
func (sc *SailPointConnector) syncIdentityAccess(ctx context.Context) error {
	log.Println("Syncing identity access (role assignments)...")

	// Get all synced identities
	rows, err := sc.pool.Query(ctx, `
		SELECT source_user_id FROM identity_sources WHERE source_system = $1
	`, sc.config.SourceSystem)
	if err != nil {
		return fmt.Errorf("query identities: %w", err)
	}
	defer rows.Close()

	identityCount := 0
	for rows.Next() {
		var sailpointID string
		if err := rows.Scan(&sailpointID); err != nil {
			continue
		}

		sc.rateLimiter.WaitIfNeeded(ctx)

		access, err := sc.client.GetIdentityAccess(ctx, sailpointID)
		if err != nil {
			sc.recordWarning(fmt.Sprintf("get access for identity %s: %v", sailpointID, err))
			continue
		}

		// Get canonical identity ID
		var canonicalID int64
		err = sc.pool.QueryRow(ctx, `
			SELECT identity_id FROM identity_sources 
			WHERE source_system = $1 AND source_user_id = $2
		`, sc.config.SourceSystem, sailpointID).Scan(&canonicalID)

		if err != nil {
			continue
		}

		// Process access items (access profiles, roles, entitlements)
		for _, item := range access {
			itemType, _ := item["type"].(string)
			itemID, _ := item["id"].(string)
			_, _ = item["name"].(string) // itemName not used but extracted for potential future use

			if itemType == "ACCESS_PROFILE" {
				// Map to role
				var roleID int64
				err = sc.pool.QueryRow(ctx, `
					SELECT id FROM roles WHERE source_system = $1 AND source_id = $2
				`, sc.config.SourceSystem, "accessprofile_"+itemID).Scan(&roleID)

				if err == nil {
					_, _ = sc.pool.Exec(ctx, `
						INSERT INTO identity_role (identity_id, role_id, source_system, source_id, synced_at, created_at)
						VALUES ($1, $2, $3, $4, NOW(), NOW())
						ON CONFLICT (identity_id, role_id, source_system) DO UPDATE SET
							synced_at = NOW()
					`, canonicalID, roleID, sc.config.SourceSystem, sailpointID+"|accessprofile_"+itemID)
				}
			} else if itemType == "ROLE" {
				// Map to role
				var roleID int64
				err = sc.pool.QueryRow(ctx, `
					SELECT id FROM roles WHERE source_system = $1 AND source_id = $2
				`, sc.config.SourceSystem, "role_"+itemID).Scan(&roleID)

				if err == nil {
					_, _ = sc.pool.Exec(ctx, `
						INSERT INTO identity_role (identity_id, role_id, source_system, source_id, synced_at, created_at)
						VALUES ($1, $2, $3, $4, NOW(), NOW())
						ON CONFLICT (identity_id, role_id, source_system) DO UPDATE SET
							synced_at = NOW()
					`, canonicalID, roleID, sc.config.SourceSystem, sailpointID+"|role_"+itemID)
				}
			} else if itemType == "ENTITLEMENT" {
				// Map to permission
				var permID int64
				err = sc.pool.QueryRow(ctx, `
					SELECT id FROM permissions WHERE source_system = $1 AND source_id = $2
				`, sc.config.SourceSystem, "entitlement_"+itemID).Scan(&permID)

				if err == nil {
					// Find role that has this permission, or create direct permission assignment
					// For now, we'll link through roles if available
					_, _ = sc.pool.Exec(ctx, `
						INSERT INTO role_permission (role_id, permission_id, source_system, source_id, synced_at, created_at)
						SELECT r.id, $1, $2, $3, NOW(), NOW()
						FROM roles r
						WHERE r.source_system = $2
						LIMIT 1
						ON CONFLICT (role_id, permission_id, source_system) DO UPDATE SET
							synced_at = NOW()
					`, permID, sc.config.SourceSystem, sailpointID+"|entitlement_"+itemID)
				}
			}
		}

		identityCount++
		if identityCount%100 == 0 {
			log.Printf("Processed %d identities...", identityCount)
		}
	}

	log.Printf("Synced access for %d identities", identityCount)
	return nil
}

// Component 3: Account Objects - Sync Linked Accounts
// Linked accounts represent accounts (how identities access systems)
// Answers: "What accounts exist?"
func (sc *SailPointConnector) syncLinkedAccounts(ctx context.Context) error {
	log.Println("Syncing linked accounts (as account objects)...")

	// Get all synced identities
	rows, err := sc.pool.Query(ctx, `
		SELECT source_user_id FROM identity_sources WHERE source_system = $1
	`, sc.config.SourceSystem)
	if err != nil {
		return fmt.Errorf("query identities: %w", err)
	}
	defer rows.Close()

	accountCount := 0
	for rows.Next() {
		var sailpointID string
		if err := rows.Scan(&sailpointID); err != nil {
			continue
		}

		sc.rateLimiter.WaitIfNeeded(ctx)

		// Get accounts for this identity (from identity access data)
		access, err := sc.client.GetIdentityAccess(ctx, sailpointID)
		if err != nil {
			sc.recordWarning(fmt.Sprintf("get access for identity %s: %v", sailpointID, err))
			continue
		}

		// Get canonical identity ID
		var canonicalID int64
		err = sc.pool.QueryRow(ctx, `
			SELECT identity_id FROM identity_sources 
			WHERE source_system = $1 AND source_user_id = $2
		`, sc.config.SourceSystem, sailpointID).Scan(&canonicalID)

		if err != nil {
			continue
		}

		// Extract linked accounts from access data
		// Access items may contain account information
		for _, item := range access {
			itemType, _ := item["type"].(string)
			if itemType == "ACCOUNT" || itemType == "ENTITLEMENT" {
				accountID, _ := item["id"].(string)
				accountName, _ := item["name"].(string)
				accountNativeID, _ := item["nativeId"].(string)

				// Store account as resource
				metadata := map[string]interface{}{
					"account_type": itemType,
					"native_id": accountNativeID,
				}
				metadataJSON, _ := json.Marshal(metadata)

				_, err = sc.pool.Exec(ctx, `
					INSERT INTO resources (name, resource_type, source_system, source_id, metadata, synced_at, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5::jsonb, NOW(), NOW(), NOW())
					ON CONFLICT (source_system, source_id) DO UPDATE SET
						name = $1,
						metadata = $5::jsonb,
						synced_at = NOW(),
						updated_at = NOW()
				`, accountName, "linked_account", sc.config.SourceSystem, "account_"+accountID, metadataJSON)

				if err == nil {
					accountCount++
				}
			}
		}
	}

	log.Printf("Synced %d linked accounts", accountCount)
	return nil
}

// Component 6: Governance Signals
// Answers: "Is there governance evidence?"
func (sc *SailPointConnector) syncGovernanceSignals(ctx context.Context) error {
	log.Println("Syncing governance signals...")

	// SailPoint governance signals:
	// 1. Access certifications (access reviews)
	// 2. SoD violations (if available via API)
	// 3. Expired certifications
	// 4. Disabled users with active access (identity drift)

	// Check for disabled users with active access (identity drift)
	rows, err := sc.pool.Query(ctx, `
		SELECT DISTINCT is.identity_id, is.source_user_id
		FROM identity_sources is
		WHERE is.source_system = $1
		  AND is.source_status = 'inactive'
		  AND EXISTS (
			SELECT 1 FROM group_roles gr
			JOIN roles r ON gr.role_id = r.id
			WHERE r.source_system = $1
			  AND gr.source_id LIKE '%' || is.source_user_id || '%'
		)
	`, sc.config.SourceSystem)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var identityID int64
			var sourceUserID string
			if err := rows.Scan(&identityID, &sourceUserID); err == nil {
				sc.recordWarning(fmt.Sprintf("Governance: Disabled identity %s has active access", sourceUserID))
			}
		}
	}

	// Check for admin roles without proper governance
	rows, err = sc.pool.Query(ctx, `
		SELECT DISTINCT i.id, i.email, r.name as role_name
		FROM identities i
		JOIN identity_sources is ON i.id = is.identity_id
		JOIN roles r ON r.source_system = is.source_system
		WHERE is.source_system = $1
		  AND r.privilege_level IN ('admin', 'elevated')
	`, sc.config.SourceSystem)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var identityID int64
			var email, roleName string
			if err := rows.Scan(&identityID, &email, &roleName); err == nil {
				// In a full implementation, check certification status
				sc.recordWarning(fmt.Sprintf("Governance: Admin user %s has role %s (check certification status)", email, roleName))
			}
		}
	}

	return nil
}

// recordError records an error
func (sc *SailPointConnector) recordError(msg string) {
	sc.errorCount++
	sc.lastError = msg
	log.Printf("ERROR: %s", msg)
}

// recordWarning records a warning
func (sc *SailPointConnector) recordWarning(msg string) {
	sc.warningCount++
	log.Printf("WARNING: %s", msg)
}

// Close closes the connector
func (sc *SailPointConnector) Close() {
	if sc.pool != nil {
		sc.pool.Close()
	}
}

// SyncManager handles sync run tracking
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
	metadataJSON, _ := json.Marshal(metadata)
	err := sm.pool.QueryRow(ctx, `
		INSERT INTO sync_runs (source_system, connector_name, status, started_at, metadata)
		VALUES ($1, $2, 'running', NOW(), $3)
		RETURNING id
	`, sourceSystem, connectorName, metadataJSON).Scan(&syncRunID)
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

// CreateEffectivePermissionSnapshot creates a snapshot of effective permissions
func (sm *SyncManager) CreateEffectivePermissionSnapshot(ctx context.Context, syncRunID int64) error {
	_, err := sm.pool.Exec(ctx, `
		DELETE FROM effective_permission_snapshots WHERE sync_run_id = $1
	`, syncRunID)
	if err != nil {
		return err
	}

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
			NULL::BIGINT as resource_id,
			NOW()
		FROM identity_effective_permissions
	`, syncRunID)
	return err
}
