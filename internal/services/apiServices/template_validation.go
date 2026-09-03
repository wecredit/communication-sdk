package apiServices

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
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

	rcsTransactionalTemplateCategory int64 = 1
	rcsPromotionalTemplateCategory   int64 = 2
	smsServiceImplicitCategory       int64 = 3
	smsServiceExplicitCategory       int64 = 4
)

var pinnacleRCSTemplateIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

var (
	ErrTemplateNotFound   = errors.New("template not found")
	ErrTemplateConflict   = errors.New("active template conflicts with an existing resolution key")
	ErrTemplateDuplicate  = errors.New("an identical template already exists")
	ErrTemplateValidation = errors.New("template validation failed")
	ErrTemplateBusy       = errors.New("template mutation is temporarily busy")
	ErrTemplateStale      = errors.New("template changed while acquiring locks")
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

// validateCreateDuplicate rejects a repeated create with identical business
// fields. IsActive is deliberately excluded: callers should update the
// existing row when only its active state needs to change.
func validateCreateDuplicate(db *gorm.DB, template apiModels.Templatedetails) error {
	query := db.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).
		Where(
			`Client = ? AND Channel = ? AND Process = ? AND Vendor = ?
			AND TemplateName = ? AND ImageId = ? AND ImageUrl = ?
			AND DltTemplateId = ? AND TemplateEntityId = ? AND TemplateHeader = ?
			AND TemplateText = ? AND Link = ? AND TemplateCategory = ?
			AND TemplateVariables = ? AND SmsFallbackVariables = ?
			AND Subject = ? AND FromEmail = ?`,
			template.Client, template.Channel, template.Process, template.Vendor,
			template.TemplateName, template.ImageId, template.ImageUrl,
			template.DltTemplateId, template.TemplateEntityId, template.TemplateHeader,
			template.TemplateText, template.Link, template.TemplateCategory,
			template.TemplateVariables, template.SmsFallbackVariables,
			template.Subject, template.FromEmail,
		)

	if template.Stage == nil {
		query = query.Where("Stage IS NULL")
	} else {
		query = query.Where("Stage = ?", *template.Stage)
	}

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
		return fmt.Errorf("check duplicate template: %w", err)
	}

	return fmt.Errorf("%w: template id %d", ErrTemplateDuplicate, existing.Id)
}

// ValidateTemplateStructure validates the template structure
func ValidateTemplateStructure(template apiModels.Templatedetails) error {
	if template.Process == "" || template.Client == "" || template.Channel == "" || template.Vendor == "" {
		return errors.New("process, client, channel, and vendor are required")
	}

	switch template.Channel {
	case "SMS", "RCS", "WHATSAPP", "EMAIL", "PUSH":
	default:
		return fmt.Errorf("unsupported channel %q", template.Channel)
	}

	switch template.Channel {
	case "RCS":
		if template.TemplateCategory != rcsTransactionalTemplateCategory &&
			template.TemplateCategory != rcsPromotionalTemplateCategory {
			return errors.New("templateCategory must be 1 (transactional) or 2 (promotional) for RCS")
		}

	case "SMS":
		if template.TemplateCategory != smsServiceImplicitCategory &&
			template.TemplateCategory != smsServiceExplicitCategory {
			return errors.New("templateCategory must be 3 (service implicit) or 4 (service explicit) for SMS")
		}

	case "PUSH":
		if strings.TrimSpace(template.TemplateHeader) == "" {
			return errors.New("templateHeader is required as the title for PUSH")
		}

		if strings.TrimSpace(template.TemplateText) == "" {
			return errors.New("templateText is required as the body for PUSH")
		}
	}

	if template.Channel == "SMS" || template.Channel == "RCS" {
		if err := validateTemplateVariablePlaceholders(template.TemplateText, template.TemplateVariables); err != nil {
			return err
		}
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

		case "RCS", "WHATSAPP", "EMAIL", "PUSH":
			if template.TemplateName == "" {
				return fmt.Errorf("templateName is required for %s REFERENCE_MODE", template.Channel)
			}
		}
	}

	if template.Channel == "RCS" {
		if template.Vendor == "PINNACLE" && !pinnacleRCSTemplateIDPattern.MatchString(template.TemplateName) {
			return errors.New("templateName must be the 24-character hexadecimal Pinnacle RCS template _id")
		}

		hasFallbackID := template.DltTemplateId > 0
		hasFallbackVariables := template.SmsFallbackVariables != ""
		if hasFallbackID != hasFallbackVariables {
			return errors.New("RCS dltTemplateId and smsFallbackVariables must both be present or both be absent")
		}
	}

	return nil
}

// validateTemplateVariablePlaceholders keeps the stored variable order aligned
// with the generic DLT placeholders consumed by the sending code. Each
// {#var#} occurrence represents exactly one comma-separated variable name.
func validateTemplateVariablePlaceholders(templateText, templateVariables string) error {
	placeholderCount := strings.Count(templateText, "{#var#}")
	variables, err := parseTemplateVariables(templateVariables)
	if err != nil {
		return err
	}

	if len(variables) != placeholderCount {
		return fmt.Errorf(
			"templateVariables contains %d entries but templateText contains %d {#var#} placeholders",
			len(variables), placeholderCount,
		)
	}

	for _, variable := range variables {
		if strings.EqualFold(variable, "var") && (len(variables) != 1 || placeholderCount != 1) {
			return errors.New(`the general template variable "var" may only be used as the single variable for one {#var#} placeholder`)
		}
	}

	return nil
}

func parseTemplateVariables(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	variables := make([]string, 0, len(parts))
	for _, part := range parts {
		variable := strings.TrimSpace(part)
		if variable == "" {
			return nil, errors.New("templateVariables must be a comma-separated list without empty entries")
		}
		variables = append(variables, variable)
	}

	return variables, nil
}

// validateStagePrerequisites ensures a stage-mode template has a resolvable
// TemplateStage mapping. A LendersStages schedule is optional: existing flows
// legitimately use mappings without a schedule. TemplateDetails.Process is the
// runtime LenderName, while decimal Stage stores Stage.SubStage (for example,
// 2.10 means Stage=2 and SubStage=10). Reference mode bypasses stage resolution.
func validateStagePrerequisites(db *gorm.DB, template apiModels.Templatedetails) error {
	if template.Stage == nil {
		return nil
	}

	canonicalStage, err := cache.CanonicalTemplateStage(*template.Stage)
	if err != nil {
		return fmt.Errorf("derive stage prerequisites: %w", err)
	}

	parts := strings.SplitN(canonicalStage, ".", 2)
	stage, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("derive whole stage from %q: %w", canonicalStage, err)
	}

	subStage, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("derive substage from %q: %w", canonicalStage, err)
	}

	var stageMappings []struct {
		ID int `gorm:"column:Id"`
	}
	if err := db.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateStageTable).
		Select("Id").
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where(
			"LenderName = ? AND CommType = ? AND Stage = ? AND SubStage = ?",
			strings.ToLower(template.Process), strings.ToUpper(template.Channel), stage, subStage,
		).
		Limit(1).
		Find(&stageMappings).Error; err != nil {
		return fmt.Errorf("check template stage prerequisite: %w", err)
	}

	if len(stageMappings) != 0 {
		return nil
	}

	return fmt.Errorf(
		"%w: stage mapping is missing for client %q, process/lender %q, channel %q, stage %s: create %s entry for Stage %d and SubStage %d before adding or updating this template",
		ErrTemplateValidation,
		template.Client,
		template.Process,
		template.Channel,
		canonicalStage,
		config.Configs.TemplateStageTable,
		stage,
		subStage,
	)
}

// validateActiveUniqueness validates the active uniqueness
func validateActiveUniqueness(db *gorm.DB, template apiModels.Templatedetails) error {
	if !template.IsActive {
		return nil
	}

	query := db.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Where("IsActive = ?", true)
	if template.Id != 0 {
		query = query.Where("Id <> ?", template.Id)
	}

	mode := templateResolutionMode(template)
	query = query.Where("Client = ? AND Channel = ? AND Vendor = ?", template.Client, template.Channel, template.Vendor)

	switch mode {
	case ResolutionModeStage:
		query = query.Where("Stage IS NOT NULL").Where("Process = ? AND Stage = ?", template.Process, *template.Stage)
	case ResolutionModeReference:
		query = query.Where("Stage IS NULL").Where("Process = ?", template.Process)
		switch template.Channel {
		case "SMS":
			query = query.Where("DltTemplateId = ?", template.DltTemplateId)
		case "WHATSAPP":
			query = query.Where("TemplateName = ?", template.TemplateName)
		case "RCS", "EMAIL", "PUSH":
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
// GET_LOCK serializes mutations targeting the same resolution key, providing
// race protection without a generated database column.
func resolutionLockName(template apiModels.Templatedetails) string {
	parts := []string{
		templateResolutionMode(template),
		strings.ToLower(template.Client),
		strings.ToLower(template.Channel),
		strings.ToLower(template.Vendor),
	}

	if template.Stage != nil {
		parts = append(parts, strings.ToLower(template.Process), strconv.FormatFloat(*template.Stage, 'f', 2, 64))
	} else {
		parts = append(parts, strings.ToLower(template.Process))
		if template.Channel == "SMS" {
			parts = append(parts, strconv.FormatInt(template.DltTemplateId, 10))
		} else {
			parts = append(parts, strings.ToLower(template.TemplateName))
		}
	}

	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("comm-template:%x", digest[:24])
}

func acquireResolutionLock(db *gorm.DB, template apiModels.Templatedetails) (string, error) {
	if !template.IsActive {
		return "", nil
	}
	return acquireNamedResolutionLock(db, template)
}

// acquireCreateResolutionLock serializes active and inactive creates so two
// concurrent identical requests cannot both pass duplicate validation.
func acquireCreateResolutionLock(db *gorm.DB, template apiModels.Templatedetails) (string, error) {
	return acquireNamedResolutionLock(db, template)
}

func acquireNamedResolutionLock(db *gorm.DB, template apiModels.Templatedetails) (string, error) {
	name := resolutionLockName(template)
	var acquired int
	if err := db.Raw("SELECT GET_LOCK(?, 10)", name).Scan(&acquired).Error; err != nil {
		return "", fmt.Errorf("acquire template uniqueness lock: %w", err)
	}

	if acquired != 1 {
		return "", fmt.Errorf("%w: timed out waiting for template uniqueness lock", ErrTemplateBusy)
	}

	return name, nil
}

// acquireTemplateMutationLock serializes updates and deletes of the same row
// before either operation discovers the row's current resolution identity.
func acquireTemplateMutationLock(db *gorm.DB, id int) (string, error) {
	if id <= 0 {
		return "", errors.New("template mutation lock requires a positive template ID")
	}

	name := fmt.Sprintf("comm-template-row:%d", id)
	var acquired int
	if err := db.Raw("SELECT GET_LOCK(?, 10)", name).Scan(&acquired).Error; err != nil {
		return "", fmt.Errorf("acquire template row lock: %w", err)
	}
	if acquired != 1 {
		return "", fmt.Errorf("%w: timed out waiting for template row lock", ErrTemplateBusy)
	}
	return name, nil
}

func releaseTemplateMutationLock(db *gorm.DB, name string) {
	releaseResolutionLock(db, name)
}

func releaseResolutionLock(db *gorm.DB, name string) {
	if name != "" {
		var released int
		_ = db.Raw("SELECT RELEASE_LOCK(?)", name).Scan(&released).Error
	}
}
