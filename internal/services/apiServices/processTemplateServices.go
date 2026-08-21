package apiServices

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

type TemplateService struct {
	ReadDB  *gorm.DB
	WriteDB *gorm.DB
}

func NewTemplateService(readDB, writeDB *gorm.DB) *TemplateService {
	return &TemplateService{ReadDB: readDB, WriteDB: writeDB}
}

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

	templates := make([]apiModels.Templatedetails, 0, params.PageSize)
	offset := (params.Page - 1) * params.PageSize
	if err := query.
		Order("Client, Channel, Process, Stage, Vendor, Id").
		Limit(params.PageSize).
		Offset(offset).
		Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}

	return &apiModels.TemplateListResult{Items: templates, TotalItems: totalItems}, nil
}

// getTemplatesFromCache preserves the previous read path for rollback while
// dashboard/select APIs use the configured read replica.
func (s *TemplateService) getTemplatesFromCache(process, stage, client, channel, vendor string) ([]apiModels.Templatedetails, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return nil, errors.New("template data not found in cache")
	}

	var templates []apiModels.Templatedetails

	// Case 1: all params provided → direct key lookup
	if process != "" && stage != "" && client != "" && channel != "" && vendor != "" {
		key := fmt.Sprintf("Process:%s|Stage:%s|Client:%s|Channel:%s|Vendor:%s", process, stage, client, channel, vendor)
		if data, ok := templateDetails[key]; ok {
			template, err := mapToTemplate(data)
			if err != nil {
				utils.Error(fmt.Errorf("failed to convert cache data to template: %v", err))
				return nil, err
			}
			return []apiModels.Templatedetails{*template}, nil
		}
		return nil, nil // no match
	}

	// Case 2: filtering
	for _, data := range templateDetails {
		stageMatches := true
		if stage != "" {
			stageValue, ok := data["Stage"].(float64)
			stageMatches = ok && fmt.Sprintf("%.2f", stageValue) == stage
		}

		if (process != "" && data["Process"] != process) ||
			!stageMatches ||
			(client != "" && data["Client"] != client) ||
			(channel != "" && data["Channel"] != channel) ||
			(vendor != "" && data["Vendor"] != vendor) {
			continue
		}
		template, err := mapToTemplate(data)
		if err != nil {
			utils.Error(fmt.Errorf("skipping invalid template data: %v", err))
			continue
		}
		templates = append(templates, *template)
	}

	// Sorting in required flow: Client > Channel > Process > Stage > Vendor
	sort.SliceStable(templates, func(i, j int) bool {
		if templates[i].Client != templates[j].Client {
			return templates[i].Client < templates[j].Client
		}
		if templates[i].Channel != templates[j].Channel {
			return templates[i].Channel < templates[j].Channel
		}
		if templates[i].Process != templates[j].Process {
			return templates[i].Process < templates[j].Process
		}
		if templates[i].Stage != nil && templates[j].Stage != nil && *templates[i].Stage != *templates[j].Stage {
			return *templates[i].Stage < *templates[j].Stage
		}
		if templates[i].Stage == nil && templates[j].Stage != nil {
			return true
		}
		if templates[i].Stage != nil && templates[j].Stage == nil {
			return false
		}
		return templates[i].Vendor < templates[j].Vendor
	})

	return templates, nil
}

func (s *TemplateService) GetTemplateByID(id uint) (*apiModels.Templatedetails, error) {
	var template apiModels.Templatedetails
	if err := s.ReadDB.Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}

		return nil, fmt.Errorf("get template %d: %w", id, err)
	}

	return &template, nil
}

// getTemplateByIDFromCache preserves the previous read path for rollback.
func (s *TemplateService) getTemplateByIDFromCache(id uint) (*apiModels.Templatedetails, error) {
	idIndex, found := cache.GetCache().GetMappedIdData(cache.TemplateDetailsData + ":IdIndex")
	if !found {
		utils.Error(fmt.Errorf("template Id index not found in cache"))
		return nil, errors.New("template Id index not found in cache")
	}

	key, ok := idIndex[id]
	if !ok {
		return nil, errors.New("template not found")
	}

	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return nil, errors.New("template data not found in cache")
	}

	data, ok := templateDetails[key]
	if !ok {
		return nil, errors.New("template not found")
	}

	template, err := mapToTemplate(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert cache data to template: %v", err))
		return nil, err
	}

	return template, nil
}

func (s *TemplateService) AddTemplate(template *apiModels.Templatedetails) error {
	istOffset := 5*time.Hour + 30*time.Minute
	template.CreatedOn = time.Now().UTC().Add(istOffset)
	normalizeTemplate(template)
	if err := ValidateTemplateStructure(*template); err != nil {
		return fmt.Errorf("%w: %v", ErrTemplateValidation, err)
	}

	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		lockName, lockErr := acquireResolutionLock(conn, *template)
		if lockErr != nil {
			return lockErr
		}
		defer releaseResolutionLock(conn, lockName)

		return conn.Transaction(func(tx *gorm.DB) error {
			if err := validateActiveUniqueness(tx, *template); err != nil {
				return err
			}
			return tx.Table(config.Configs.TemplateDetailsTable).Create(template).Error
		})
	})

	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.WriteDB)

	return nil
}

func (s *TemplateService) UpdateTemplateById(id int, updates apiModels.TemplateUpdateRequest) (*apiModels.Templatedetails, error) {
	var saved apiModels.Templatedetails
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		var lockName string
		defer func() { releaseResolutionLock(conn, lockName) }()

		return conn.Transaction(func(tx *gorm.DB) error {
			if err := tx.Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&saved).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTemplateNotFound
				}
				return fmt.Errorf("load template %d for update: %w", id, err)
			}

			updates.Apply(&saved)
			normalizeTemplate(&saved)
			if err := ValidateTemplateStructure(saved); err != nil {
				return fmt.Errorf("%w: %v", ErrTemplateValidation, err)
			}

			var err error
			lockName, err = acquireResolutionLock(tx, saved)
			if err != nil {
				return err
			}

			if err := validateActiveUniqueness(tx, saved); err != nil {
				return err
			}

			istOffset := 5*time.Hour + 30*time.Minute
			now := time.Now().UTC().Add(istOffset)
			saved.UpdatedOn = &now

			return tx.Table(config.Configs.TemplateDetailsTable).Save(&saved).Error
		})
	})

	if err != nil {
		return nil, err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.WriteDB)
	return &saved, nil
}

func (s *TemplateService) DeleteTemplate(id int) error {
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		var lockName string
		defer func() { releaseResolutionLock(conn, lockName) }()

		return conn.Transaction(func(tx *gorm.DB) error {
			var existing apiModels.Templatedetails
			if err := tx.Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).First(&existing).Error; err != nil {
				return err
			}

			normalizeTemplate(&existing)
			var err error
			lockName, err = acquireResolutionLock(tx, existing)
			if err != nil {
				return err
			}

			result := tx.Table(config.Configs.TemplateDetailsTable).Where("Id = ?", id).Delete(&apiModels.Templatedetails{})
			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}

			return nil
		})
	})

	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.WriteDB)
	return nil
}
func mapToTemplate(data map[string]interface{}) (*apiModels.Templatedetails, error) {
	if data == nil {
		return nil, fmt.Errorf("input data is nil")
	}

	getStr := func(key string) string {
		if val, ok := data[key].(string); ok {
			return val
		}
		return ""
	}

	getInt := func(key string) int {
		if val, ok := data[key].(int64); ok {
			return int(val)
		}
		return 0
	}

	getFloat := func(key string) float64 {
		switch val := data[key].(type) {
		case float64:
			return val
		case []byte:
			f, err := strconv.ParseFloat(string(val), 64)
			if err == nil {
				return f
			}
		case string:
			f, err := strconv.ParseFloat(val, 64)
			if err == nil {
				return f
			}
		}
		return 0
	}

	getOptionalFloat := func(key string) *float64 {
		if data[key] == nil {
			return nil
		}
		value := getFloat(key)
		return &value
	}

	getBool := func(key string) bool {
		if val, ok := data[key].(int64); ok {
			return val == 1
		}
		return false
	}

	template := &apiModels.Templatedetails{
		Id:                   getInt("Id"),
		Client:               getStr("Client"),
		Channel:              getStr("Channel"),
		Process:              getStr("Process"),
		Stage:                getOptionalFloat("Stage"),
		Vendor:               getStr("Vendor"),
		TemplateName:         getStr("TemplateName"),
		ImageId:              getStr("ImageId"),
		ImageUrl:             getStr("ImageUrl"),
		DltTemplateId:        int64(getInt("DltTemplateId")), // stored as int64 anyway
		TemplateEntityId:     int64(getInt("TemplateEntityId")),
		TemplateHeader:       getStr("TemplateHeader"),
		IsActive:             getBool("IsActive"),
		TemplateText:         getStr("TemplateText"),
		TemplateCategory:     int64(getInt("TemplateCategory")),
		TemplateVariables:    getStr("TemplateVariables"),
		SmsFallbackVariables: getStr("SmsFallbackVariables"),
		FromEmail:            getStr("FromEmail"),
		Subject:              getStr("Subject"),
		Link:                 getStr("Link"),
	}
	// CreatedOn
	if createdOn, ok := data["CreatedOn"].(time.Time); ok {
		template.CreatedOn = createdOn
	}

	// UpdatedOn
	if raw, ok := data["UpdatedOn"]; ok && raw != nil {
		switch v := raw.(type) {
		case time.Time:
			template.UpdatedOn = &v
		case string:
			layout := "2006-01-02 15:04:05.999 +0000 UTC"
			if parsed, err := time.Parse(layout, v); err == nil {
				template.UpdatedOn = &parsed
			} else {
				return nil, fmt.Errorf("invalid UpdatedOn time format: %v", err)
			}
		default:
			return nil, fmt.Errorf("unsupported type for UpdatedOn: %T", raw)
		}
	}

	return template, nil
}
