package apiServices

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ResolutionModeStage     = "STAGE_MODE"
	ResolutionModeReference = "REFERENCE_MODE"
)

var (
	ErrTemplateNotFound   = errors.New("template not found")
	ErrTemplateConflict   = errors.New("active template conflicts with an existing resolution key")
	ErrTemplateValidation = errors.New("template validation failed")
	ErrTemplateBusy       = errors.New("template mutation is temporarily busy")
)

// normalizeTemplate normalizes the template
func normalizeTemplate(template *apiModels.Templatedetails) {
	template.Process = strings.ToUpper(strings.TrimSpace(template.Process))
	template.Channel = strings.ToUpper(strings.TrimSpace(template.Channel))
	template.Vendor = strings.ToUpper(strings.TrimSpace(template.Vendor))
	template.Client = strings.ToLower(strings.TrimSpace(template.Client))
	template.TemplateName = strings.TrimSpace(template.TemplateName)
	template.TemplateVariables = strings.TrimSpace(template.TemplateVariables)
	template.SmsFallbackVariables = strings.TrimSpace(template.SmsFallbackVariables)
}

// ValidateTemplateStructure validates the template structure
func ValidateTemplateStructure(template apiModels.Templatedetails) error {
	if template.Process == "" || template.Client == "" || template.Channel == "" || template.Vendor == "" {
		return errors.New("process, client, channel, and vendor are required")
	}

	switch template.Channel {
	case "SMS", "RCS", "WHATSAPP", "EMAIL":
	default:
		return fmt.Errorf("unsupported channel %q", template.Channel)
	}

	mode := templateResolutionMode(template)
	switch mode {
	case ResolutionModeStage:
		if *template.Stage < 0 {
			return errors.New("stage cannot be negative")
		}

		if _, err := cache.CanonicalTemplateStage(*template.Stage); err != nil {
			return fmt.Errorf("invalid stage: %w", err)
		}

	case ResolutionModeReference:
		switch template.Channel {
		case "SMS":
			if template.DltTemplateId <= 0 {
				return errors.New("dltTemplateId is required for SMS REFERENCE_MODE")
			}

		case "RCS", "WHATSAPP", "EMAIL":
			if template.TemplateName == "" {
				return fmt.Errorf("templateName is required for %s REFERENCE_MODE", template.Channel)
			}
		}
	}

	if template.Channel == "RCS" {
		hasFallbackID := template.DltTemplateId > 0
		hasFallbackVariables := template.SmsFallbackVariables != ""
		if hasFallbackID != hasFallbackVariables {
			return errors.New("RCS dltTemplateId and smsFallbackVariables must both be present or both be absent")
		}
	}

	return nil
}

// validateActiveUniqueness validates the active uniqueness
func validateActiveUniqueness(db *gorm.DB, template apiModels.Templatedetails) error {
	if !template.IsActive {
		return nil
	}

	query := db.Table(config.Configs.TemplateDetailsTable).Where("IsActive = ?", true)
	if template.Id != 0 {
		query = query.Where("Id <> ?", template.Id)
	}

	mode := templateResolutionMode(template)
	query = query.Where("Client = ? AND Channel = ? AND Vendor = ?", template.Client, template.Channel, template.Vendor)

	switch mode {
	case ResolutionModeStage:
		query = query.Where("Stage IS NOT NULL").Where("Process = ? AND Stage = ?", template.Process, *template.Stage)
	case ResolutionModeReference:
		query = query.Where("Stage IS NULL")
		switch template.Channel {
		case "SMS":
			query = query.Where("DltTemplateId = ?", template.DltTemplateId)
		case "WHATSAPP":
			query = query.Where("TemplateName = ?", template.TemplateName)
		case "RCS", "EMAIL":
			query = query.Where("TemplateName = ?", template.TemplateName)
		}
	}

	// A locking read is deliberate here. UpdateTemplateById loads the row before
	// acquiring the advisory lock, so a plain SELECT would reuse an older
	// REPEATABLE READ snapshot and could miss a competing commit made while
	// GET_LOCK was waiting. InnoDB locking reads use the latest committed state.
	var existing struct {
		Id int `gorm:"column:Id"`
	}

	err := query.
		Select("Id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Limit(1).
		Take(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("check active template uniqueness: %w", err)
	}

	return ErrTemplateConflict
}

func templateResolutionMode(template apiModels.Templatedetails) string {
	if template.Stage != nil {
		return ResolutionModeStage
	}
	return ResolutionModeReference
}

// resolutionLockName returns a stable, non-sensitive MySQL advisory-lock name.
// GET_LOCK serializes only mutations targeting the same active resolution key,
// providing race protection without a generated database column.
func resolutionLockName(template apiModels.Templatedetails) string {
	parts := []string{
		templateResolutionMode(template),
		strings.ToLower(template.Client),
		strings.ToLower(template.Channel),
		strings.ToLower(template.Vendor),
	}

	if template.Stage != nil {
		parts = append(parts, strings.ToLower(template.Process), strconv.FormatFloat(*template.Stage, 'f', 2, 64))
	} else if template.Channel == "SMS" {
		parts = append(parts, strconv.FormatInt(template.DltTemplateId, 10))
	} else {
		parts = append(parts, strings.ToLower(template.TemplateName))
	}

	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("comm-template:%x", digest[:24])
}

func acquireResolutionLock(db *gorm.DB, template apiModels.Templatedetails) (string, error) {
	if !template.IsActive {
		return "", nil
	}

	name := resolutionLockName(template)
	var acquired int
	if err := db.Raw("SELECT GET_LOCK(?, 10)", name).Scan(&acquired).Error; err != nil {
		return "", fmt.Errorf("acquire active-template lock: %w", err)
	}

	if acquired != 1 {
		return "", fmt.Errorf("%w: timed out waiting for active-template uniqueness lock", ErrTemplateBusy)
	}

	return name, nil
}

func releaseResolutionLock(db *gorm.DB, name string) {
	if name != "" {
		var released int
		_ = db.Raw("SELECT RELEASE_LOCK(?)", name).Scan(&released).Error
	}
}
