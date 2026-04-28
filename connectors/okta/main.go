package okta

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	okta "github.com/okta/okta-sdk-golang/v4/okta"
)

// OktaConnector syncs data from Okta to canonical schema
type OktaConnector struct {
	config          *Config
	pool            *pgxpool.Pool
	oktaClient      *okta.APIClient
	resolution      *IdentityResolution
	rateLimiter     *RateLimiter
	syncManager     *SyncManager
	errorCount      int
	warningCount    int
	lastError       string
}

// NewOktaConnector creates a new Okta connector
func NewOktaConnector(cfg *Config) (*OktaConnector, error) {
	// Validate config
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.OktaDomain == "" {
		return nil, fmt.Errorf("OKTA_DOMAIN is required")
	}
	if cfg.OktaAPIToken == "" {
		return nil, fmt.Errorf("OKTA_API_TOKEN is required")
	}

	// Connect to database
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database connection: %w", err)
	}

	// Create Okta client
	oktaConfig, err := okta.NewConfiguration(okta.WithOrgUrl("https://"+cfg.OktaDomain), okta.WithToken(cfg.OktaAPIToken))
	if err != nil {
		return nil, fmt.Errorf("create okta config: %w", err)
	}
	oktaClient := okta.NewAPIClient(oktaConfig)

	// Create sync manager (reuse from backend)
	syncMgr := NewSyncManager(pool)

	// Load privilege markers (Component 5: Privilege Signals)
	if err := LoadPrivilegeMarkers(""); err != nil {
		log.Printf("Warning: Could not load privilege markers: %v", err)
	}

	return &OktaConnector{
		config:       cfg,
		pool:         pool,
		oktaClient:   oktaClient,
		resolution:  &IdentityResolution{pool: pool, minConfidence: cfg.MinConfidenceScore},
		rateLimiter:  NewRateLimiter(cfg.MaxRetries, cfg.RetryBackoff, cfg.RateLimitWindow),
		syncManager:  syncMgr,
	}, nil
}

// Sync runs a full sync from Okta
func (oc *OktaConnector) Sync(ctx context.Context) error {
	log.Println("Starting Okta sync...")

	// Start sync run
	syncRunID, err := oc.syncManager.StartSyncRun(ctx, oc.config.SourceSystem, oc.config.ConnectorName, map[string]interface{}{
		"incremental": oc.config.IncrementalSync,
		"changed_since": oc.config.ChangedSince,
	})
	if err != nil {
		return fmt.Errorf("start sync run: %w", err)
	}

	defer func() {
		status := "success"
		if oc.errorCount > 0 {
			status = "partial"
		}
		if err != nil {
			status = "error"
		}
		oc.syncManager.FinishSyncRun(ctx, syncRunID, status, oc.errorCount, oc.warningCount, &oc.lastError)
	}()

	// Sync users
	if err := oc.syncUsers(ctx); err != nil {
		oc.recordError(fmt.Sprintf("sync users: %v", err))
		return err
	}

	// Sync groups
	if err := oc.syncGroups(ctx); err != nil {
		oc.recordError(fmt.Sprintf("sync groups: %v", err))
		return err
	}

	// Sync apps (as roles)
	if err := oc.syncApps(ctx); err != nil {
		oc.recordError(fmt.Sprintf("sync apps: %v", err))
		return err
	}

	// Component 4: Authorization Layer - Sync Okta Admin Roles
	if err := oc.syncOktaRoles(ctx); err != nil {
		oc.recordError(fmt.Sprintf("sync Okta roles: %v", err))
		// Don't fail sync if roles fail, but log it
	}

	// Component 3: Account Objects - Sync App Assignments
	if err := oc.syncAppAssignments(ctx); err != nil {
		oc.recordError(fmt.Sprintf("sync app assignments: %v", err))
		// Don't fail sync if app assignments fail
	}

	// Component 6: Governance Signals
	if err := oc.syncGovernanceSignals(ctx); err != nil {
		oc.recordWarning(fmt.Sprintf("sync governance signals: %v", err))
		// Governance is optional
	}

	// Create effective permission snapshot
	if err := oc.syncManager.CreateEffectivePermissionSnapshot(ctx, syncRunID); err != nil {
		oc.recordWarning(fmt.Sprintf("create snapshot: %v", err))
	}

	log.Printf("Sync complete. Errors: %d, Warnings: %d", oc.errorCount, oc.warningCount)

	// Report low-confidence matches
	lowConf := oc.resolution.GetLowConfidenceMatches()
	if len(lowConf) > 0 {
		log.Printf("⚠️  %d low-confidence identity matches need manual review", len(lowConf))
		for _, match := range lowConf {
			log.Printf("  - Okta user %s matched to identity %d (confidence: %.2f, method: %s)",
				match.OktaUserID, match.CanonicalID, match.Confidence, match.MatchMethod)
		}
	}

	return nil
}

// syncUsers syncs Okta users to canonical identities
func (oc *OktaConnector) syncUsers(ctx context.Context) error {
	log.Println("Syncing users...")

	usersAPI := oc.oktaClient.UserAPI
	limit := int32(200) // Okta max page size
	after := ""
	totalSynced := 0

	for {
		oc.rateLimiter.WaitIfNeeded(ctx)

		var users []okta.User
		var err error

		err = oc.rateLimiter.RetryWithBackoff(ctx, func() error {
			var listResp []okta.User
			if oc.config.IncrementalSync && oc.config.ChangedSince != nil {
				// Use search API for incremental sync
				query := fmt.Sprintf("lastUpdated gt \"%s\"", oc.config.ChangedSince.Format(time.RFC3339))
				listResp, _, err = usersAPI.ListUsers(ctx).
					Limit(limit).
					After(after).
					Search(query).
					Execute()
			} else {
				listResp, _, err = usersAPI.ListUsers(ctx).
					Limit(limit).
					After(after).
					Execute()
			}

			if err != nil {
				return err
			}

			users = listResp
			return nil
		})

		if err != nil {
			return fmt.Errorf("list users: %w", err)
		}

		if len(users) == 0 {
			break
		}

		for _, user := range users {
			if err := oc.upsertUser(ctx, user); err != nil {
				if user.Id != nil {
					oc.recordError(fmt.Sprintf("upsert user %s: %v", *user.Id, err))
				} else {
					oc.recordError(fmt.Sprintf("upsert user: %v", err))
				}
				continue
			}
			totalSynced++
		}

		// Check if there are more pages
		if len(users) < int(limit) {
			break
		}
		if len(users) > 0 && users[len(users)-1].Id != nil {
			after = *users[len(users)-1].Id
		} else {
			break
		}
	}

	log.Printf("Synced %d users", totalSynced)
	return nil
}

// upsertUser upserts a single Okta user
func (oc *OktaConnector) upsertUser(ctx context.Context, user okta.User) error {
	// Extract user fields
	if user.Id == nil {
		return fmt.Errorf("user has no ID")
	}
	userID := *user.Id
	email := ""
	displayName := ""
	employeeID := ""
	status := "active"
	upn := ""

	if user.Profile != nil {
		if user.Profile.Email != nil {
			email = *user.Profile.Email
		}
		if user.Profile.DisplayName.IsSet() {
			if val := user.Profile.DisplayName.Get(); val != nil {
				displayName = *val
			}
		} else {
			firstName := ""
			lastName := ""
			if user.Profile.FirstName.IsSet() {
				if val := user.Profile.FirstName.Get(); val != nil {
					firstName = *val
				}
			}
			if user.Profile.LastName.IsSet() {
				if val := user.Profile.LastName.Get(); val != nil {
					lastName = *val
				}
			}
			if firstName != "" || lastName != "" {
				displayName = firstName + " " + lastName
			}
		}
		if user.Profile.EmployeeNumber != nil {
			employeeID = *user.Profile.EmployeeNumber
		}
		// UserPrincipalName is not in Okta UserProfile - use email as UPN
		upn = email
	}

	if user.Status != nil {
		status = string(*user.Status)
		// Normalize status
		if status == "ACTIVE" {
			status = "active"
		} else if status == "DEPROVISIONED" || status == "SUSPENDED" {
			status = "inactive"
		}
	}

	if email == "" {
		return fmt.Errorf("user %s has no email", userID)
	}

	// Resolve identity
	canonicalID, confidence, _, err := oc.resolution.ResolveIdentity(ctx, userID, email, employeeID, upn)
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}

	// Create new identity if not found
	if canonicalID == 0 {
		canonicalID, err = oc.resolution.CreateOrGetIdentity(ctx, email, displayName, employeeID, status)
		if err != nil {
			return fmt.Errorf("create identity: %w", err)
		}
	}

	// Check MFA status from Okta (try to get factors, but don't fail if it doesn't work)
	hasMFA := false
	if oc.oktaClient.UserFactorAPI != nil {
		oc.rateLimiter.WaitIfNeeded(ctx)
		factors, _, err := oc.oktaClient.UserFactorAPI.ListFactors(ctx, userID).Execute()
		if err == nil && len(factors) > 0 {
			// Check if any active MFA factors exist
			for _, factor := range factors {
				// Okta SDK v4 uses GetStatus() and GetFactorType() methods
				status := factor.GetStatus()
				if status != nil && *status == "ACTIVE" {
					factorType := factor.GetFactorType()
					if factorType != nil {
						factorTypeStr := string(*factorType)
						// Common MFA factor types
						if factorTypeStr == "push" || factorTypeStr == "sms" || factorTypeStr == "token" || 
						   factorTypeStr == "token:software:totp" || factorTypeStr == "token:hardware" ||
						   factorTypeStr == "webauthn" || factorTypeStr == "okta_verify" {
							hasMFA = true
							break
						}
					}
				}
			}
		}
	}

	// Build metadata JSON with MFA status
	metadata := map[string]interface{}{
		"mfa_enabled":    hasMFA,
		"mfa_configured": hasMFA,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Upsert identity_sources with MFA metadata
	_, err = oc.pool.Exec(ctx, `
		INSERT INTO identity_sources (identity_id, source_system, source_user_id, source_status, confidence, metadata, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_user_id) DO UPDATE SET
			identity_id = $1,
			source_status = $4,
			confidence = $5,
			metadata = $6::jsonb,
			synced_at = NOW(),
			updated_at = NOW()
	`, canonicalID, oc.config.SourceSystem, userID, status, confidence, metadataJSON)

	return err
}

// syncGroups syncs Okta groups
func (oc *OktaConnector) syncGroups(ctx context.Context) error {
	log.Println("Syncing groups...")

	groupsAPI := oc.oktaClient.GroupAPI
	limit := int32(200)
	after := ""

	for {
		oc.rateLimiter.WaitIfNeeded(ctx)

		var groups []okta.Group
		var err error

		err = oc.rateLimiter.RetryWithBackoff(ctx, func() error {
			listResp, _, err := groupsAPI.ListGroups(ctx).
				Limit(limit).
				After(after).
				Execute()

			if err != nil {
				return err
			}

			groups = listResp
			return nil
		})

		if err != nil {
			return fmt.Errorf("list groups: %w", err)
		}

		if len(groups) == 0 {
			break
		}

		for _, group := range groups {
			if err := oc.upsertGroup(ctx, group); err != nil {
				if group.Id != nil {
					oc.recordError(fmt.Sprintf("upsert group %s: %v", *group.Id, err))
				} else {
					oc.recordError(fmt.Sprintf("upsert group: %v", err))
				}
				continue
			}
		}

		if len(groups) < int(limit) {
			break
		}
		if len(groups) > 0 && groups[len(groups)-1].Id != nil {
			after = *groups[len(groups)-1].Id
		} else {
			break
		}
	}

	return nil
}

// upsertGroup upserts a single Okta group
func (oc *OktaConnector) upsertGroup(ctx context.Context, group okta.Group) error {
	if group.Id == nil {
		return fmt.Errorf("group has no ID")
	}
	groupID := *group.Id
	name := ""
	if group.Profile != nil && group.Profile.Name != nil {
		name = *group.Profile.Name
	}

	if name == "" {
		name = groupID
	}

	_, err := oc.pool.Exec(ctx, `
		INSERT INTO groups (name, source_system, source_id, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_id) DO UPDATE SET
			name = $1,
			synced_at = NOW(),
			updated_at = NOW()
	`, name, oc.config.SourceSystem, groupID)

	if err != nil {
		return err
	}

	// Sync group memberships
	return oc.syncGroupMembers(ctx, groupID)
}

// syncGroupMembers syncs group memberships
func (oc *OktaConnector) syncGroupMembers(ctx context.Context, groupID string) error {
	groupsAPI := oc.oktaClient.GroupAPI
	limit := int32(200)
	after := ""

	for {
		oc.rateLimiter.WaitIfNeeded(ctx)

		var users []okta.User
		var err error

		err = oc.rateLimiter.RetryWithBackoff(ctx, func() error {
			listResp, _, err := groupsAPI.ListGroupUsers(ctx, groupID).
				Limit(limit).
				After(after).
				Execute()

			if err != nil {
				return err
			}

			users = listResp
			return nil
		})

		if err != nil {
			return err
		}

		if len(users) == 0 {
			break
		}

		// Get canonical group ID
		var canonicalGroupID int64
		err = oc.pool.QueryRow(ctx, `
			SELECT id FROM groups WHERE source_system = $1 AND source_id = $2
		`, oc.config.SourceSystem, groupID).Scan(&canonicalGroupID)

		if err != nil {
			oc.recordWarning(fmt.Sprintf("group not found: %s", groupID))
			break
		}

		for _, user := range users {
			if user.Id == nil {
				continue
			}
			userID := *user.Id
			
			// Get canonical identity ID from identity_sources
			var canonicalIdentityID int64
			err = oc.pool.QueryRow(ctx, `
				SELECT identity_id FROM identity_sources 
				WHERE source_system = $1 AND source_user_id = $2
			`, oc.config.SourceSystem, userID).Scan(&canonicalIdentityID)

			if err != nil {
				continue // User not synced yet
			}

			// Upsert identity_group
			_, err = oc.pool.Exec(ctx, `
				INSERT INTO identity_group (identity_id, group_id, source_system, source_id, synced_at, created_at)
				VALUES ($1, $2, $3, $4, NOW(), NOW())
				ON CONFLICT (identity_id, group_id, source_system) DO UPDATE SET
					synced_at = NOW()
			`, canonicalIdentityID, canonicalGroupID, oc.config.SourceSystem, groupID+"|"+userID)
		}

		if len(users) < int(limit) {
			break
		}
		if len(users) > 0 && users[len(users)-1].Id != nil {
			after = *users[len(users)-1].Id
		} else {
			break
		}
	}

	return nil
}

// syncApps syncs Okta apps as roles
func (oc *OktaConnector) syncApps(ctx context.Context) error {
	log.Println("Syncing apps (as roles)...")

	appsAPI := oc.oktaClient.ApplicationAPI
	limit := int32(200)
	after := ""

	for {
		oc.rateLimiter.WaitIfNeeded(ctx)

		var apps []okta.ListApplications200ResponseInner
		var err error

		err = oc.rateLimiter.RetryWithBackoff(ctx, func() error {
			listResp, _, err := appsAPI.ListApplications(ctx).
				Limit(limit).
				After(after).
				Execute()

			if err != nil {
				return err
			}

			apps = listResp
			return nil
		})

		if err != nil {
			return fmt.Errorf("list apps: %w", err)
		}

		if len(apps) == 0 {
			break
		}

		for _, appResp := range apps {
			// Convert ListApplications200ResponseInner to Application using JSON
			// This works because both types have compatible JSON structures
			appJSON, err := json.Marshal(appResp)
			if err != nil {
				oc.recordWarning(fmt.Sprintf("failed to marshal app: %v", err))
				continue
			}
			
			var app okta.Application
			if err := json.Unmarshal(appJSON, &app); err != nil {
				oc.recordWarning(fmt.Sprintf("failed to unmarshal app: %v", err))
				continue
			}
			
			if err := oc.upsertApp(ctx, app); err != nil {
				if app.Id != nil {
					oc.recordError(fmt.Sprintf("upsert app %s: %v", *app.Id, err))
				} else {
					oc.recordError(fmt.Sprintf("upsert app: %v", err))
				}
				continue
			}
		}

		if len(apps) < int(limit) {
			break
		}
		// For pagination, get the last app's ID
		if len(apps) > 0 {
			// Convert last app to get its ID
			lastAppJSON, _ := json.Marshal(apps[len(apps)-1])
			var lastApp okta.Application
			if err := json.Unmarshal(lastAppJSON, &lastApp); err == nil && lastApp.Id != nil {
				after = *lastApp.Id
			} else {
				break
			}
		} else {
			break
		}
	}

	return nil
}

// upsertApp upserts an Okta app as a role
func (oc *OktaConnector) upsertApp(ctx context.Context, app okta.Application) error {
	if app.Id == nil {
		return fmt.Errorf("app has no ID")
	}
	appID := *app.Id
	name := ""
	if app.Label != nil {
		name = *app.Label
	}
	if name == "" {
		name = appID
	}

	// Component 5: Privilege Signals - Use privilege markers to determine privilege level
	appName := name
	// Application.Name might not exist in SDK v4, use Label or SignOnMode instead
	// Label is already used above, so just use name
	privilegeLevel := string(GetPrivilegeLevel(appName))
	if privilegeLevel == "standard" {
		// Fallback: Check app type for privilege hints
		if app.SignOnMode != nil && *app.SignOnMode == "SAML_2_0" {
			privilegeLevel = "elevated" // SAML apps often have elevated access
		} else {
			privilegeLevel = "read"
		}
	}

	_, err := oc.pool.Exec(ctx, `
		INSERT INTO roles (name, privilege_level, source_system, source_id, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (source_system, source_id) DO UPDATE SET
			name = $1,
			privilege_level = $2,
			synced_at = NOW(),
			updated_at = NOW()
	`, name, privilegeLevel, oc.config.SourceSystem, appID)

	return err
}

// Component 3: Account Objects - Sync App Assignments
// App assignments represent accounts (how identities access applications)
// Answers: "What accounts exist?"
func (oc *OktaConnector) syncAppAssignments(ctx context.Context) error {
	log.Println("Syncing app assignments (as account objects)...")

	appsAPI := oc.oktaClient.ApplicationAPI
	limit := int32(200)
	after := ""

	for {
		oc.rateLimiter.WaitIfNeeded(ctx)

		var apps []okta.ListApplications200ResponseInner
		var err error

		err = oc.rateLimiter.RetryWithBackoff(ctx, func() error {
			listResp, _, err := appsAPI.ListApplications(ctx).
				Limit(limit).
				After(after).
				Execute()

			if err != nil {
				return err
			}

			apps = listResp
			return nil
		})

		if err != nil {
			return fmt.Errorf("list apps: %w", err)
		}

		if len(apps) == 0 {
			break
		}

		for _, appResp := range apps {
			appJSON, err := json.Marshal(appResp)
			if err != nil {
				continue
			}
			
			var app okta.Application
			if err := json.Unmarshal(appJSON, &app); err != nil {
				continue
			}

			if app.Id == nil {
				continue
			}

			// Get app assignments (users assigned to app)
			if err := oc.syncAppUserAssignments(ctx, *app.Id); err != nil {
				oc.recordWarning(fmt.Sprintf("sync assignments for app %s: %v", *app.Id, err))
			}
		}

		if len(apps) < int(limit) {
			break
		}
		if len(apps) > 0 {
			lastAppJSON, _ := json.Marshal(apps[len(apps)-1])
			var lastApp okta.Application
			if err := json.Unmarshal(lastAppJSON, &lastApp); err == nil && lastApp.Id != nil {
				after = *lastApp.Id
			} else {
				break
			}
		} else {
			break
		}
	}

	return nil
}

// syncAppUserAssignments syncs user assignments to an app (account objects)
func (oc *OktaConnector) syncAppUserAssignments(ctx context.Context, appID string) error {
	// Note: App user assignments functionality temporarily disabled
	// due to Okta SDK v4 API method differences
	// This can be re-enabled once the correct SDK method is identified
	oc.recordWarning(fmt.Sprintf("App user assignments skipped for app %s (SDK v4 API method needs verification)", appID))
	return nil
}

// Component 4: Authorization Layer - Sync Okta Admin Roles
// Syncs Okta admin roles (SUPER_ADMIN, ORG_ADMIN, etc.) and their assignments to users
func (oc *OktaConnector) syncOktaRoles(ctx context.Context) error {
	log.Println("Syncing Okta admin roles...")

	// First, sync the role definitions (SUPER_ADMIN, ORG_ADMIN, etc.)
	if err := oc.syncOktaRoleDefinitions(ctx); err != nil {
		return fmt.Errorf("sync role definitions: %w", err)
	}

	// Then, sync user role assignments
	if err := oc.syncOktaUserRoleAssignments(ctx); err != nil {
		return fmt.Errorf("sync user role assignments: %w", err)
	}

	return nil
}

// syncOktaRoleDefinitions syncs Okta role types as roles in the database
func (oc *OktaConnector) syncOktaRoleDefinitions(ctx context.Context) error {
	// Okta has predefined admin roles
	oktaRoles := []struct {
		name           string
		privilegeLevel string
	}{
		{"SUPER_ADMIN", "admin"},
		{"ORG_ADMIN", "admin"},
		{"API_ACCESS_MANAGEMENT_ADMIN", "admin"},
		{"GROUP_ADMIN", "elevated"},
		{"APP_ADMIN", "elevated"},
		{"USER_ADMIN", "elevated"},
		{"READ_ONLY_ADMIN", "elevated"},
		{"HELP_DESK_ADMIN", "elevated"},
	}

	for _, role := range oktaRoles {
		// Use privilege markers to determine privilege level
		privilegeLevel := string(GetPrivilegeLevel(role.name))
		if privilegeLevel == "standard" {
			privilegeLevel = role.privilegeLevel
		}

		_, err := oc.pool.Exec(ctx, `
			INSERT INTO roles (name, privilege_level, source_system, source_id, synced_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
			ON CONFLICT (source_system, source_id) DO UPDATE SET
				name = $1,
				privilege_level = $2,
				synced_at = NOW(),
				updated_at = NOW()
		`, role.name, privilegeLevel, oc.config.SourceSystem, "okta_role_"+role.name)
		if err != nil {
			return fmt.Errorf("upsert role %s: %w", role.name, err)
		}
	}

	return nil
}

// syncOktaUserRoleAssignments syncs which users have which Okta admin roles
func (oc *OktaConnector) syncOktaUserRoleAssignments(ctx context.Context) error {
	// Get all users from identity_sources for this system
	rows, err := oc.pool.Query(ctx, `
		SELECT identity_id, source_user_id 
		FROM identity_sources 
		WHERE source_system = $1
	`, oc.config.SourceSystem)
	if err != nil {
		return fmt.Errorf("get users: %w", err)
	}
	defer rows.Close()

	assignedCount := 0
	for rows.Next() {
		var canonicalIdentityID int64
		var sourceUserID string
		if err := rows.Scan(&canonicalIdentityID, &sourceUserID); err != nil {
			continue
		}

		oc.rateLimiter.WaitIfNeeded(ctx)

		// Try to get roles assigned to this user using User API
		// Note: Okta SDK v4 uses UserAPI.ListAssignedRolesForUser
		userAPI := oc.oktaClient.UserAPI
		if userAPI == nil {
			continue
		}

		// Get user roles - Okta SDK v4 method
		// Note: The exact method name depends on Okta SDK v4 version
		// TODO: Implement using the correct Okta SDK v4 API method
		// Common method names: ListAssignedRolesForUser, ListUserRoles, GetAssignedRoles
		// For now, skip role assignment sync - this needs to be implemented with the correct SDK method
		oc.recordWarning(fmt.Sprintf("Role assignments skipped for user %s (SDK v4 API method needs verification)", sourceUserID))
		continue
	}

	log.Printf("Assigned %d Okta admin roles to users", assignedCount)
	return nil
}

// Component 6: Governance Signals
// Answers: "Is there governance evidence?"
func (oc *OktaConnector) syncGovernanceSignals(ctx context.Context) error {
	log.Println("Syncing governance signals...")

	// Okta governance signals:
	// 1. Disabled users with active app assignments (identity drift)
	// 2. Users without MFA in admin roles (policy violation)
	// 3. Stale app assignments (not used in 90+ days)

	// Check for disabled users with active assignments
	rows, err := oc.pool.Query(ctx, `
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
	`, oc.config.SourceSystem)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var identityID int64
			var sourceUserID string
			if err := rows.Scan(&identityID, &sourceUserID); err == nil {
				oc.recordWarning(fmt.Sprintf("Governance: Disabled user %s has active assignments", sourceUserID))
			}
		}
	}

	// Check for admin users without MFA
	rows, err = oc.pool.Query(ctx, `
		SELECT DISTINCT i.id, i.email
		FROM identities i
		JOIN identity_sources is ON i.id = is.identity_id
		JOIN roles r ON r.source_system = is.source_system
		WHERE is.source_system = $1
		  AND r.privilege_level IN ('admin', 'elevated')
		  AND (is.metadata->>'mfa_enabled' IS NULL OR is.metadata->>'mfa_enabled' = 'false')
	`, oc.config.SourceSystem)

	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var identityID int64
			var email string
			if err := rows.Scan(&identityID, &email); err == nil {
				oc.recordWarning(fmt.Sprintf("Governance: Admin user %s without MFA", email))
			}
		}
	}

	return nil
}

// recordError records an error
func (oc *OktaConnector) recordError(msg string) {
	oc.errorCount++
	oc.lastError = msg
	log.Printf("ERROR: %s", msg)
}

// recordWarning records a warning
func (oc *OktaConnector) recordWarning(msg string) {
	oc.warningCount++
	log.Printf("WARNING: %s", msg)
}

// Close closes the connector
func (oc *OktaConnector) Close() {
	if oc.pool != nil {
		oc.pool.Close()
	}
}

// SyncManager is a simplified version for the connector
type SyncManager struct {
	pool *pgxpool.Pool
}

func NewSyncManager(pool *pgxpool.Pool) *SyncManager {
	return &SyncManager{pool: pool}
}

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
