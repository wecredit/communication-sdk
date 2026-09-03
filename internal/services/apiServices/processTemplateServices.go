package apiServices

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	internalredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TemplateService struct {
	ReadDB  *gorm.DB
	WriteDB *gorm.DB
}

func NewTemplateService(readDB, writeDB *gorm.DB) *TemplateService {
	return &TemplateService{ReadDB: readDB, WriteDB: writeDB}
}

// GetTemplates gets the templates from the database
func (s *TemplateService) GetTemplates(params apiModels.TemplateListParams) (*apiModels.TemplateListResult, error) {
	query := s.ReadDB.Table(config.Configs.TemplateDetailsTable)
	if params.Process != "" {
		query = query.Where("Process = ?", params.Process)
	}

	if params.Stage != "" {
		query = query.Where("Stage = CAST(? AS DECIMAL(10,2))", params.Stage)
	}

	if params.Client != "" {
		query = query.Where("Client = ?", params.Client)
	}

	if params.Channel != "" {
		query = query.Where("Channel = ?", params.Channel)
	}

	if params.Vendor != "" {
		query = query.Where("Vendor = ?", params.Vendor)
	}

	var totalItems int64
	if err := query.Count(&totalItems).Error; err != nil {
		return nil, fmt.Errorf("count templates: %w", err)
	}

	templates := make([]apiModels.TemplateListItem, 0, params.PageSize)
	offset := (params.Page - 1) * params.PageSize

	if err := query.
		Select("Id, Client, Channel, Process, CAST(Stage AS CHAR) AS Stage, Vendor, TemplateName, DltTemplateId, IsActive, CreatedOn, COALESCE(UpdatedOn, CreatedOn) AS UpdatedOn, CreatedBy, UpdatedBy").
		Order("COALESCE(UpdatedOn, CreatedOn) DESC, Id DESC").
		Limit(params.PageSize).
		Offset(offset).
		Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	return &apiModels.TemplateListResult{Items: templates, TotalItems: totalItems}, nil
}

// GetTemplateByID gets a template by its ID
func (s *TemplateService) GetTemplateByID(id uint) (*apiModels.Templatedetails, error) {
	var template apiModels.Templatedetails

	// get the template from the database
	if err := s.ReadDB.Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}

		return nil, fmt.Errorf("get template %d: %w", id, err)
	}

	return &template, nil
}

// AddTemplate adds a template
func (s *TemplateService) AddTemplate(template *apiModels.Templatedetails, actorUsername string) error {
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	template.CreatedOn = now
	template.UpdatedOn = &now
	template.CreatedBy = actorUsername
	template.UpdatedBy = actorUsername
	normalizeTemplate(template)
	if err := ValidateTemplateStructure(*template); err != nil {
		return fmt.Errorf("%w: %v", ErrTemplateValidation, err)
	}

	var invalidationVersion int64
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		var lockName string
		var stageLocks []string
		defer func() {
			releaseResolutionLock(conn, lockName)
			releaseStageConfigurationLocks(conn, stageLocks)
		}()

		stageIdentity, err := templateStageLockIdentity(*template)
		if err != nil {
			return fmt.Errorf("derive template stage lock: %w", err)
		}

		stageLocks, err = acquireStageConfigurationLocks(conn, stageIdentity)
		if err != nil {
			return err
		}
		lockName, err = acquireCreateResolutionLock(conn, *template)
		if err != nil {
			return err
		}

		return conn.Session(&gorm.Session{NewDB: true}).Transaction(func(tx *gorm.DB) error {
			if err := validateStagePrerequisites(tx, *template); err != nil {
				return err
			}

			if err := validateCreateDuplicate(tx, *template); err != nil {
				return err
			}

			if err := validateActiveUniqueness(tx, *template); err != nil {
				return err
			}

			if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Create(template).Error; err != nil {
				return fmt.Errorf("create template: %w", err)
			}

			if !template.IsActive {
				return nil
			}

			invalidationVersion, err = configurationcache.IncrementTemplateVersion(tx, config.Configs.ConfigurationVersionTable)
			return err
		})
	})

	if err != nil {
		return err
	}

	if invalidationVersion == 0 {
		utils.Info(fmt.Sprintf(
			"template cache reload not required (operation=create templateId=%d reason=template-is-inactive)",
			template.Id,
		))
		return nil
	}

	publishTemplateInvalidation(invalidationVersion)
	return nil
}

// UpdateTemplateById updates a template by its ID
func (s *TemplateService) UpdateTemplateById(id int, updates apiModels.TemplateUpdateRequest, actorUsername string) (*apiModels.Templatedetails, error) {
	var saved apiModels.Templatedetails
	var invalidationVersion int64
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		var lockName string
		var mutationLockName string
		var stageLocks []string
		defer func() {
			releaseResolutionLock(conn, lockName)
			releaseStageConfigurationLocks(conn, stageLocks)
			releaseTemplateMutationLock(conn, mutationLockName)
		}()

		var err error
		mutationLockName, err = acquireTemplateMutationLock(conn, id)
		if err != nil {
			return err
		}

		var discovered apiModels.Templatedetails
		if err := conn.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&discovered).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTemplateNotFound
			}
			return fmt.Errorf("discover template %d for update: %w", id, err)
		}

		saved = discovered
		updates.Apply(&saved)
		normalizeTemplate(&saved)
		if err := ValidateTemplateStructure(saved); err != nil {
			return fmt.Errorf("%w: %v", ErrTemplateValidation, err)
		}

		stageIdentity, err := templateStageLockIdentity(saved)
		if err != nil {
			return fmt.Errorf("derive template stage lock: %w", err)
		}

		stageLocks, err = acquireStageConfigurationLocks(conn, stageIdentity)
		if err != nil {
			return err
		}
		lockName, err = acquireResolutionLock(conn, saved)
		if err != nil {
			return err
		}

		return conn.Session(&gorm.Session{NewDB: true}).Transaction(func(tx *gorm.DB) error {
			var current apiModels.Templatedetails
			if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("Id = ?", id).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTemplateNotFound
				}
				return fmt.Errorf("lock template %d for update: %w", id, err)
			}
			if !reflect.DeepEqual(current, discovered) {
				return ErrTemplateStale
			}

			wasActive := current.IsActive
			if err := validateStagePrerequisites(tx, saved); err != nil {
				return err
			}

			if err := validateActiveUniqueness(tx, saved); err != nil {
				return err
			}

			istOffset := 5*time.Hour + 30*time.Minute
			now := time.Now().UTC().Add(istOffset)
			saved.UpdatedOn = &now
			saved.UpdatedBy = actorUsername
			result := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).
				Select(
					"Client", "Channel", "Process", "Stage", "Vendor", "TemplateName",
					"ImageId", "ImageUrl", "DltTemplateId", "TemplateEntityId", "TemplateHeader",
					"IsActive", "TemplateText", "Link", "UpdatedOn", "UpdatedBy", "TemplateCategory",
					"TemplateVariables", "SmsFallbackVariables", "Subject", "FromEmail",
				).
				Updates(&saved)
			if result.Error != nil {
				return fmt.Errorf("update template %d: %w", id, result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrTemplateStale
			}

			if !wasActive && !saved.IsActive {
				return nil
			}

			invalidationVersion, err = configurationcache.IncrementTemplateVersion(tx, config.Configs.ConfigurationVersionTable)
			return err
		})
	})

	if err != nil {
		return nil, err
	}

	publishTemplateInvalidation(invalidationVersion)
	return &saved, nil
}

// DeleteTemplate deletes a template
func (s *TemplateService) DeleteTemplate(id int) error {
	var invalidationVersion int64
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		var lockName string
		var mutationLockName string
		defer func() {
			releaseResolutionLock(conn, lockName)
			releaseTemplateMutationLock(conn, mutationLockName)
		}()

		var err error
		mutationLockName, err = acquireTemplateMutationLock(conn, id)
		if err != nil {
			return err
		}

		var discovered apiModels.Templatedetails
		if err := conn.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&discovered).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTemplateNotFound
			}
			return fmt.Errorf("discover template %d for delete: %w", id, err)
		}

		lockName, err = acquireResolutionLock(conn, discovered)
		if err != nil {
			return err
		}

		return conn.Session(&gorm.Session{NewDB: true}).Transaction(func(tx *gorm.DB) error {
			var current apiModels.Templatedetails
			if err := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("Id = ?", id).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTemplateNotFound
				}
				return fmt.Errorf("lock template %d for delete: %w", id, err)
			}
			if !reflect.DeepEqual(current, discovered) {
				return ErrTemplateStale
			}

			result := tx.Session(&gorm.Session{NewDB: true}).Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).Delete(&apiModels.Templatedetails{})
			if result.Error != nil {
				return result.Error
			}

			// check if the rows affected is 0
			if result.RowsAffected == 0 {
				return ErrTemplateStale
			}

			if !current.IsActive {
				return nil
			}

			invalidationVersion, err = configurationcache.IncrementTemplateVersion(tx, config.Configs.ConfigurationVersionTable)
			return err
		})
	})

	if err != nil {
		return err
	}

	publishTemplateInvalidation(invalidationVersion)
	return nil
}

// publishTemplateInvalidation publishes a template invalidation event
func publishTemplateInvalidation(templateVersion int64) {
	if templateVersion <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// publish the template invalidation event
	if err := configurationcache.PublishTemplateInvalidation(ctx, internalredis.RDB, config.Configs.Environment, templateVersion); err != nil {
		// The transaction is already committed. Version polling recovers a missed event.
		utils.Error(err)
		return
	}

	utils.Info(fmt.Sprintf(
		"template cache invalidation published (environment=%s templateVersion=%d)",
		config.Configs.Environment, templateVersion,
	))
}
