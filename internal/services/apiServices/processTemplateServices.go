package apiServices

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

type TemplateService struct {
	DB *gorm.DB
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{DB: db}
}

func (s *TemplateService) GetTemplates(process, stage, client, channel, vendor string) ([]apiModels.Templatedetails, error) {
	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB) // Temporary: ensure cache is populated
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return nil, errors.New("template data not found in cache")
	}

	var templates []apiModels.Templatedetails

	// Case 1: both process and stage and channel and vendor provided -> direct key lookup
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
		if (process != "" && data["Process"] != process) || (stage != "" && data["Stage"] != stage) {
			continue
		}
		template, err := mapToTemplate(data)
		if err != nil {
			utils.Error(fmt.Errorf("skipping invalid template data: %v", err))
			continue
		}
		templates = append(templates, *template)
	}

	return templates, nil
}

func (s *TemplateService) GetTemplateByID(id uint) (*apiModels.Templatedetails, error) {
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
	template.Process = strings.ToUpper(template.Process)
	template.Channel = strings.ToUpper(template.Channel)
	template.Vendor = strings.ToUpper(template.Vendor)
	template.Client = strings.ToLower(template.Client)

	err := s.DB.Create(template).Error
	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB)

	return nil
}

func (s *TemplateService) UpdateTemplateByNameAndChannel(process, stage, channel, vendor, client string, updates apiModels.Templatedetails) error {
	var existing apiModels.Templatedetails
	if err := s.DB.Where("Process = ? AND Stage = ? AND Channel = ? AND Vendor = ? AND Client = ?", process, stage, channel, vendor, client).First(&existing).Error; err != nil {
		return errors.New("template not found")
	}

	existing.IsActive = updates.IsActive
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	existing.UpdatedOn = &now

	err := s.DB.Save(&existing).Error
	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB)
	return nil
}

func (s *TemplateService) DeleteTemplate(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Templatedetails{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB)
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

	getBool := func(key string) bool {
		if val, ok := data[key].(int64); ok {
			return val == 1
		}
		return false
	}

	template := &apiModels.Templatedetails{
		Id:                getInt("Id"),
		TemplateName:      getStr("TemplateName"),
		ImageId:           getStr("ImageId"),
		Process:           getStr("Process"),
		Stage:             getInt("Stage"),
		ImageUrl:          getStr("ImageUrl"),
		DltTemplateId:     int64(getInt("DltTemplateId")), // stored as int64 anyway
		Channel:           getStr("Channel"),
		Vendor:            getStr("Vendor"),
		IsActive:          getBool("IsActive"),
		TemplateText:      getStr("TemplateText"),
		Client:            getStr("Client"),
		TemplateVariables: getStr("TemplateVariables"),
		Link:              getStr("Link"),
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
