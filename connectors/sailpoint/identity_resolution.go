package sailpoint

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdentityResolution handles matching SailPoint identities to canonical identities
type IdentityResolution struct {
	pool          *pgxpool.Pool
	minConfidence float64
	lowConfidence []LowConfidenceMatch
}

// LowConfidenceMatch represents a match that needs manual review
type LowConfidenceMatch struct {
	SailPointUserID  string
	SailPointEmail   string
	SailPointEmpID   string
	CanonicalID      int64
	CanonicalEmail   string
	CanonicalEmpID   string
	Confidence       float64
	MatchMethod      string // "employee_id", "email"
}

// ResolveIdentity matches a SailPoint identity to a canonical identity
// Returns: (canonical_identity_id, confidence_score, match_method, error)
func (ir *IdentityResolution) ResolveIdentity(ctx context.Context, sailpointID, email, employeeID string) (int64, float64, string, error) {
	// Method 1: Match by employee_id (highest confidence: 1.0)
	if employeeID != "" {
		var canonicalID int64
		err := ir.pool.QueryRow(ctx, `
			SELECT id FROM identities WHERE employee_id = $1
		`, employeeID).Scan(&canonicalID)

		if err == nil {
			ir.recordMatch(sailpointID, canonicalID, 1.0, "employee_id", email, employeeID)
			return canonicalID, 1.0, "employee_id", nil
		} else if err != pgx.ErrNoRows {
			return 0, 0, "", err
		}
	}

	// Method 2: Match by email (high confidence: 0.9)
	if email != "" {
		var canonicalID int64
		var canonicalEmail string
		err := ir.pool.QueryRow(ctx, `
			SELECT id, email FROM identities WHERE LOWER(email) = LOWER($1)
		`, email).Scan(&canonicalID, &canonicalEmail)

		if err == nil {
			// Exact email match
			confidence := 0.9
			if strings.EqualFold(email, canonicalEmail) {
				confidence = 0.95
			}
			ir.recordMatch(sailpointID, canonicalID, confidence, "email", email, employeeID)
			return canonicalID, confidence, "email", nil
		} else if err != pgx.ErrNoRows {
			return 0, 0, "", err
		}
	}

	// No match found - will create new identity
	return 0, 0, "", nil
}

// CreateOrGetIdentity creates a new canonical identity or returns existing
func (ir *IdentityResolution) CreateOrGetIdentity(ctx context.Context, email, displayName, employeeID, status string) (int64, error) {
	// Check if already exists (by email)
	var existingID int64
	err := ir.pool.QueryRow(ctx, `
		SELECT id FROM identities WHERE LOWER(email) = LOWER($1)
	`, email).Scan(&existingID)

	if err == nil {
		return existingID, nil
	} else if err != pgx.ErrNoRows {
		return 0, err
	}

	// Create new identity
	var newID int64
	err = ir.pool.QueryRow(ctx, `
		INSERT INTO identities (employee_id, email, display_name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id
	`, sqlNullString(employeeID), email, sqlNullString(displayName), status).Scan(&newID)

	return newID, err
}

// GetLowConfidenceMatches returns matches that need manual review
func (ir *IdentityResolution) GetLowConfidenceMatches() []LowConfidenceMatch {
	return ir.lowConfidence
}

// recordMatch records a match, flagging low-confidence ones
func (ir *IdentityResolution) recordMatch(sailpointID string, canonicalID int64, confidence float64, method, email, employeeID string) {
	if confidence < ir.minConfidence {
		var canonicalEmail, canonicalEmpID string
		ctx := context.Background()
		_ = ir.pool.QueryRow(ctx, `
			SELECT email, employee_id FROM identities WHERE id = $1
		`, canonicalID).Scan(&canonicalEmail, &canonicalEmpID)

		ir.lowConfidence = append(ir.lowConfidence, LowConfidenceMatch{
			SailPointUserID: sailpointID,
			SailPointEmail:  email,
			SailPointEmpID: employeeID,
			CanonicalID:    canonicalID,
			CanonicalEmail: canonicalEmail,
			CanonicalEmpID: canonicalEmpID,
			Confidence:     confidence,
			MatchMethod:    method,
		})
	}
}

func sqlNullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
