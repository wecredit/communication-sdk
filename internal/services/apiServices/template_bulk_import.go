package apiServices

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"gorm.io/gorm"
)

func (s *TemplateService) BulkImportTemplates(ctx context.Context, r io.Reader, dryRun bool, actor string) (*BulkTemplateImportResult, error) {
	maxRows, _ := strconv.Atoi(config.Configs.TemplateBulkImportMaxRows)
	if maxRows <= 0 {
		maxRows = 2000
	}
	maxBytes, _ := strconv.ParseInt(config.Configs.TemplateBulkImportMaxBytes, 10, 64)
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024
	}
	rows, err := ParseBulkTemplateCSV(r, maxRows, maxBytes)
	if err != nil {
		return nil, err
	}
	result := &BulkTemplateImportResult{DryRun: dryRun, TotalRows: len(rows), CreatedTemplateIds: []int{}, Errors: []BulkTemplateError{}}
	valid := make([]BulkTemplateRow, 0, len(rows))
	identities := map[string]int{}
	activeIdentities := map[string]int{}
	for _, row := range rows {
		t := row.Template
		normalizeTemplate(&t)
		if err := ValidateTemplateStructure(t); err != nil {
			result.Errors = append(result.Errors, bulkRowError(row.Row, err))
			continue
		}
		identity := bulkTemplateIdentity(t)
		if prior, ok := identities[identity]; ok {
			result.Errors = append(result.Errors, BulkTemplateError{Row: row.Row, Code: "TEMPLATE_DUPLICATE", Message: fmt.Sprintf("duplicates row %d", prior), Field: "Template Name"})
			continue
		}
		if t.IsActive {
			activeIdentity := bulkActiveIdentity(t)
			if prior, ok := activeIdentities[activeIdentity]; ok {
				result.Errors = append(result.Errors, BulkTemplateError{Row: row.Row, Code: "TEMPLATE_CONFLICT", Message: fmt.Sprintf("active template conflicts with row %d", prior)})
				continue
			}
			activeIdentities[activeIdentity] = row.Row
		}
		identities[identity] = row.Row
		row.Template = t
		valid = append(valid, row)
	}
	result.ValidRows = len(valid)
	result.FailedRows = len(result.Errors)
	if dryRun {
		return result, nil
	}
	if len(valid) == 0 {
		return result, errors.New("bulk import contains no valid rows")
	}
	var inserted int
	var activeInserted int
	var invalidationVersion int64
	err = s.WriteDB.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		writeLockName, writeLockErr := acquireTemplateWriteLock(conn)
		if writeLockErr != nil {
			return writeLockErr
		}
		defer releaseTemplateWriteLock(conn, writeLockName)

		stageIdentities := make([]string, 0, len(valid))
		for _, row := range valid {
			identity, lockErr := templateStageLockIdentity(row.Template)
			if lockErr != nil {
				return lockErr
			}
			stageIdentities = append(stageIdentities, identity)
		}
		stageLocks, lockErr := acquireStageConfigurationLocks(conn, stageIdentities...)
		if lockErr != nil {
			return lockErr
		}
		defer releaseStageConfigurationLocks(conn, stageLocks)
		return conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			validationIndex, indexErr := buildBulkValidationIndex(tx, valid)
			if indexErr != nil {
				return indexErr
			}
			validated := make([]apiModels.Templatedetails, 0, len(valid))
			for _, row := range valid {
				t := row.Template
				now := adminNow()
				t.CreatedOn = now
				t.UpdatedOn = &now
				t.CreatedBy = actor
				t.UpdatedBy = actor
				if err := validateBulkTemplateFromIndex(validationIndex, t); err != nil {
					if isBulkRowError(err) {
						result.Errors = append(result.Errors, bulkRowError(row.Row, err))
						continue
					}
					return err
				}
				validated = append(validated, t)
			}
			if len(validated) == 0 {
				return errors.New("bulk import contains no valid rows")
			}
			if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).CreateInBatches(&validated, 200).Error; err != nil {
				return fmt.Errorf("batch create templates: %w", err)
			}
			inserted = len(validated)
			for _, t := range validated {
				if t.IsActive {
					activeInserted++
				}
				result.CreatedTemplateIds = append(result.CreatedTemplateIds, t.Id)
			}
			if activeInserted > 0 {
				var versionErr error
				invalidationVersion, versionErr = configurationcache.IncrementTemplateVersion(tx, config.Configs.ConfigurationVersionTable)
				if versionErr != nil {
					return versionErr
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	result.InsertedRows = inserted
	result.ValidRows = inserted
	result.FailedRows = len(result.Errors)
	if invalidationVersion > 0 {
		publishTemplateInvalidation(invalidationVersion)
	}
	return result, nil
}

// bulkValidationIndex keeps the expensive duplicate, active-conflict, and
// stage-prerequisite checks in memory. The previous implementation issued up
// to three locking SELECTs per CSV row, which made a 1,400-row import slow
// enough to hit the Gateway timeout.
type bulkValidationIndex struct {
	duplicates map[string]int
	active     map[string]struct{}
	stages     map[string]struct{}
}

type bulkTemplateScope struct {
	Client  string
	Channel string
	Vendor  string
}

type bulkStageMapping struct {
	LenderName string `gorm:"column:LenderName"`
	CommType   string `gorm:"column:CommType"`
	Stage      int    `gorm:"column:Stage"`
	SubStage   int    `gorm:"column:SubStage"`
}

func buildBulkValidationIndex(tx *gorm.DB, rows []BulkTemplateRow) (*bulkValidationIndex, error) {
	index := &bulkValidationIndex{
		duplicates: make(map[string]int),
		active:     make(map[string]struct{}),
		stages:     make(map[string]struct{}),
	}

	scopes := make(map[bulkTemplateScope]struct{}, len(rows))
	processes := make(map[string]struct{}, len(rows))
	channels := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		t := row.Template
		scopes[bulkTemplateScope{Client: t.Client, Channel: t.Channel, Vendor: t.Vendor}] = struct{}{}
		if t.Stage != nil {
			processes[strings.ToLower(strings.TrimSpace(t.Process))] = struct{}{}
			channels[strings.ToUpper(strings.TrimSpace(t.Channel))] = struct{}{}
		}
	}

	if len(scopes) > 0 {
		var existing []apiModels.Templatedetails
		query := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable)
		first := true
		for scope := range scopes {
			condition := "Client = ? AND Channel = ? AND Vendor = ?"
			if first {
				query = query.Where(condition, scope.Client, scope.Channel, scope.Vendor)
				first = false
			} else {
				query = query.Or(condition, scope.Client, scope.Channel, scope.Vendor)
			}
		}
		if err := query.Find(&existing).Error; err != nil {
			return nil, fmt.Errorf("prefetch existing templates: %w", err)
		}
		for _, template := range existing {
			normalizeTemplate(&template)
			index.duplicates[bulkTemplateIdentity(template)] = template.Id
			if template.IsActive {
				index.active[bulkActiveIdentity(template)] = struct{}{}
			}
		}
	}

	if len(processes) > 0 {
		processList := make([]string, 0, len(processes))
		for process := range processes {
			processList = append(processList, process)
		}
		channelList := make([]string, 0, len(channels))
		for channel := range channels {
			channelList = append(channelList, channel)
		}
		var mappings []bulkStageMapping
		if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateStageTable).
			Where("LenderName IN ? AND CommType IN ?", processList, channelList).
			Find(&mappings).Error; err != nil {
			return nil, fmt.Errorf("prefetch template stage mappings: %w", err)
		}
		for _, mapping := range mappings {
			key := stageConfigurationLockIdentity(mapping.LenderName, mapping.CommType, mapping.Stage) + fmt.Sprintf("|%d", mapping.SubStage)
			index.stages[key] = struct{}{}
		}
	}

	return index, nil
}

func validateBulkTemplateFromIndex(index *bulkValidationIndex, template apiModels.Templatedetails) error {
	if template.Stage != nil {
		canonical, err := cache.CanonicalTemplateStage(*template.Stage)
		if err != nil {
			return fmt.Errorf("derive stage prerequisites: %w", err)
		}
		parts := strings.SplitN(canonical, ".", 2)
		whole, _ := strconv.Atoi(parts[0])
		subStage, _ := strconv.Atoi(parts[1])
		key := stageConfigurationLockIdentity(template.Process, template.Channel, whole) + fmt.Sprintf("|%d", subStage)
		if _, ok := index.stages[key]; !ok {
			return fmt.Errorf("%w: stage mapping is missing for client %q, process/lender %q, channel %q, stage %s: create %s entry for Stage %d and SubStage %d before adding or updating this template", ErrTemplateValidation, template.Client, template.Process, template.Channel, canonical, config.Configs.TemplateStageTable, whole, subStage)
		}
	}

	if existingID, ok := index.duplicates[bulkTemplateIdentity(template)]; ok {
		return fmt.Errorf("%w: template id %d", ErrTemplateDuplicate, existingID)
	}
	if template.IsActive {
		if _, ok := index.active[bulkActiveIdentity(template)]; ok {
			return ErrTemplateConflict
		}
	}
	return nil
}

func validateAndInsertTemplate(tx *gorm.DB, template *apiModels.Templatedetails) error {
	if err := validateTemplateForCreate(tx, template); err != nil {
		return err
	}
	if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Create(template).Error; err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	return nil
}

func validateTemplateForCreate(tx *gorm.DB, template *apiModels.Templatedetails) error {
	if err := validateStagePrerequisites(tx, *template); err != nil {
		return err
	}
	if err := validateCreateDuplicate(tx, *template); err != nil {
		return err
	}
	if err := validateActiveUniqueness(tx, *template); err != nil {
		return err
	}
	return nil
}

func isBulkRowError(err error) bool {
	return errors.Is(err, ErrTemplateValidation) || errors.Is(err, ErrTemplateDuplicate) || errors.Is(err, ErrTemplateConflict) || errors.Is(err, ErrConfigurationBusy)
}
func bulkRowError(row int, err error) BulkTemplateError {
	code := "TEMPLATE_VALIDATION_FAILED"
	if errors.Is(err, ErrTemplateDuplicate) {
		code = "TEMPLATE_DUPLICATE"
	}
	if errors.Is(err, ErrTemplateConflict) {
		code = "TEMPLATE_CONFLICT"
	}
	return BulkTemplateError{Row: row, Code: code, Message: strings.TrimSpace(err.Error())}
}
func bulkTemplateIdentity(t apiModels.Templatedetails) string {
	stage := ""
	if t.Stage != nil {
		stage = fmt.Sprintf("%.2f", *t.Stage)
	}
	return strings.Join([]string{
		t.Client, t.Channel, t.Process, stage, t.Vendor, t.TemplateName,
		t.ImageId, t.ImageUrl, strconv.FormatInt(t.DltTemplateId, 10),
		strconv.FormatInt(t.TemplateEntityId, 10), t.TemplateHeader, t.TemplateText,
		t.Link, strconv.FormatInt(t.TemplateCategory, 10), t.TemplateVariables,
		t.SmsFallbackVariables, t.Subject, t.FromEmail, t.AppId, t.LanguageCode,
		t.CampaignId, t.CtaId, t.WabaNumber,
	}, "\x00")
}

func bulkActiveIdentity(t apiModels.Templatedetails) string {
	key := fmt.Sprintf("%s|%s|%s|%s|",
		strings.ToLower(strings.TrimSpace(t.Client)),
		strings.ToUpper(strings.TrimSpace(t.Channel)),
		strings.ToUpper(strings.TrimSpace(t.Vendor)),
		strings.ToLower(strings.TrimSpace(t.Process)))
	if t.Stage != nil {
		return key + "stage|" + fmt.Sprintf("%.2f", *t.Stage)
	}
	if t.Channel == "SMS" {
		return key + "reference|" + strconv.FormatInt(t.DltTemplateId, 10)
	}
	return key + "reference|" + strings.ToLower(strings.TrimSpace(t.TemplateName)) + "|" + strings.ToLower(strings.TrimSpace(t.AppId))
}
