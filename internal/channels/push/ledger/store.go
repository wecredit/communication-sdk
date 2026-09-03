package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var safeTableName = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

type Identity struct {
	Client              string
	EventID             string
	Campaign            string
	CampaignDate        string
	Variant             string
	EligibilityIdentity string
}

type Store struct {
	db        *gorm.DB
	tableName string
	now       func() time.Time
}

func NewStore(db *gorm.DB, tableName string) (*Store, error) {
	if db == nil {
		return nil, errors.New("PUSH ledger database is required")
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" || !safeTableName.MatchString(tableName) {
		return nil, errors.New("PUSH ledger table name is invalid")
	}
	return &Store{db: db, tableName: tableName, now: time.Now}, nil
}

// CreateOrGetPending inserts one idempotency row per delivery identity and
// token fingerprint. The manually provisioned table must enforce the matching
// composite unique key.
func (s *Store) CreateOrGetPending(ctx context.Context, identity Identity, deviceToken string) (Dispatch, bool, error) {
	identity = normalizeIdentity(identity)
	if err := validateIdentity(identity); err != nil {
		return Dispatch{}, false, err
	}
	fingerprint, err := FingerprintToken(deviceToken)
	if err != nil {
		return Dispatch{}, false, err
	}

	now := s.now().UTC()
	dispatch := Dispatch{
		Client:              identity.Client,
		EventID:             identity.EventID,
		Campaign:            identity.Campaign,
		CampaignDate:        identity.CampaignDate,
		Variant:             identity.Variant,
		EligibilityIdentity: identity.EligibilityIdentity,
		TokenFingerprint:    fingerprint,
		Status:              StatusPending,
		CreatedOn:           now,
		UpdatedOn:           now,
	}

	result := s.db.WithContext(ctx).Table(s.tableName).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&dispatch)
	if result.Error != nil {
		return Dispatch{}, false, fmt.Errorf("create PUSH ledger row: %w", result.Error)
	}
	created := result.RowsAffected == 1
	if created {
		return dispatch, true, nil
	}

	if err := s.identityQuery(ctx, identity, fingerprint).Take(&dispatch).Error; err != nil {
		return Dispatch{}, false, fmt.Errorf("load existing PUSH ledger row: %w", err)
	}
	return dispatch, false, nil
}

// Claim atomically claims pending/retryable work or reclaims an expired claim.
func (s *Store) Claim(ctx context.Context, id uint64) (Dispatch, bool, error) {
	if id == 0 {
		return Dispatch{}, false, errors.New("PUSH ledger id is required")
	}
	now := s.now().UTC()

	result := s.db.WithContext(ctx).Table(s.tableName).
		Where("Id = ? AND Status IN ?", id, []Status{StatusPending, StatusRetryable}).
		Updates(map[string]interface{}{
			"Status":    StatusClaimed,
			"ClaimedAt": now,
			"UpdatedOn": now,
		})
	if result.Error != nil {
		return Dispatch{}, false, fmt.Errorf("claim PUSH ledger row: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		result = s.db.WithContext(ctx).Table(s.tableName).
			Where("Id = ? AND Status = ? AND ClaimedAt < ?", id, StatusClaimed, now.Add(-ClaimExpiry)).
			Updates(map[string]interface{}{
				"ClaimedAt":    now,
				"ReclaimCount": gorm.Expr("ReclaimCount + 1"),
				"UpdatedOn":    now,
			})
		if result.Error != nil {
			return Dispatch{}, false, fmt.Errorf("reclaim expired PUSH ledger row: %w", result.Error)
		}
	}
	if result.RowsAffected == 0 {
		return Dispatch{}, false, nil
	}

	var dispatch Dispatch
	if err := s.db.WithContext(ctx).Table(s.tableName).Where("Id = ?", id).Take(&dispatch).Error; err != nil {
		return Dispatch{}, false, fmt.Errorf("load claimed PUSH ledger row: %w", err)
	}
	return dispatch, true, nil
}

func (s *Store) IsClaimCurrent(ctx context.Context, id uint64) (bool, time.Duration, error) {
	var dispatch Dispatch
	if err := s.db.WithContext(ctx).Table(s.tableName).
		Select("Status", "ClaimedAt").Where("Id = ?", id).Take(&dispatch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("check PUSH ledger claim: %w", err)
	}
	if dispatch.Status != StatusClaimed || dispatch.ClaimedAt == nil {
		return false, 0, nil
	}
	age := s.now().UTC().Sub(dispatch.ClaimedAt.UTC())
	return age >= 0 && age < ClaimExpiry, age, nil
}

func (s *Store) RecordAttempt(ctx context.Context, id uint64, attemptCount int) (bool, error) {
	if id == 0 || attemptCount < 1 {
		return false, errors.New("PUSH ledger id and positive attempt count are required")
	}
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Table(s.tableName).
		Where("Id = ? AND Status = ? AND ClaimedAt >= ?", id, StatusClaimed, now.Add(-ClaimExpiry)).
		Updates(map[string]interface{}{
			"AttemptCount":  attemptCount,
			"LastAttemptAt": now,
			"UpdatedOn":     now,
		})
	if result.Error != nil {
		return false, fmt.Errorf("record PUSH provider attempt: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (s *Store) Finalize(ctx context.Context, id uint64, status Status, attemptCount int, errorCode, providerMessageID string) (bool, error) {
	if id == 0 || !status.Terminal() {
		return false, errors.New("PUSH ledger id and terminal status are required")
	}
	if attemptCount < 0 || attemptCount > 2 {
		return false, errors.New("PUSH ledger attempt count must be between 0 and 2")
	}
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Table(s.tableName).
		Where("Id = ? AND Status = ? AND ClaimedAt >= ?", id, StatusClaimed, now.Add(-ClaimExpiry)).
		Updates(map[string]interface{}{
			"Status":            status,
			"AttemptCount":      attemptCount,
			"ErrorCode":         strings.TrimSpace(errorCode),
			"ProviderMessageId": strings.TrimSpace(providerMessageID),
			"FinalizedAt":       now,
			"UpdatedOn":         now,
		})
	if result.Error != nil {
		return false, fmt.Errorf("finalize PUSH ledger row: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func FingerprintToken(deviceToken string) (string, error) {
	deviceToken = strings.TrimSpace(deviceToken)
	if deviceToken == "" {
		return "", errors.New("PUSH device token is required")
	}
	digest := sha256.Sum256([]byte(deviceToken))
	return hex.EncodeToString(digest[:]), nil
}

func (s *Store) identityQuery(ctx context.Context, identity Identity, fingerprint string) *gorm.DB {
	return s.db.WithContext(ctx).Table(s.tableName).Where(
		"Client = ? AND EventId = ? AND Campaign = ? AND CampaignDate = ? AND Variant = ? AND TokenFingerprint = ?",
		identity.Client, identity.EventID, identity.Campaign, identity.CampaignDate, identity.Variant, fingerprint,
	)
}

func normalizeIdentity(identity Identity) Identity {
	identity.Client = strings.ToLower(strings.TrimSpace(identity.Client))
	identity.EventID = strings.TrimSpace(identity.EventID)
	identity.Campaign = strings.TrimSpace(identity.Campaign)
	identity.CampaignDate = strings.TrimSpace(identity.CampaignDate)
	identity.Variant = strings.TrimSpace(identity.Variant)
	identity.EligibilityIdentity = strings.TrimSpace(identity.EligibilityIdentity)
	return identity
}

func validateIdentity(identity Identity) error {
	if identity.Client == "" {
		return errors.New("PUSH ledger client is required")
	}
	if identity.EventID == "" {
		return errors.New("PUSH ledger event id is required")
	}
	if identity.Campaign == "" {
		return errors.New("PUSH ledger campaign is required")
	}
	return nil
}
