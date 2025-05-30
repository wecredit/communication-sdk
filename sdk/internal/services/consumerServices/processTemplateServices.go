package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

type TemplateService struct {
	DB *gorm.DB
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{DB: db}
}

func (s *TemplateService) GetTemplates(process, stage, channel, vendor string) ([]apiModels.Templatedetails, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return nil, errors.New("template data not found in cache")
	}

	fmt.Println("Template DEtails", templateDetails)

	var templates []apiModels.Templatedetails

	// Case 1: both process and stage and channel and vendor provided -> direct key lookup
	if process != "" && stage != "" && channel != "" && vendor != "" {
		key := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s", process, stage, channel, vendor)
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

	err := s.DB.Create(template).Error
	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB)

	return nil
}

func (s *TemplateService) UpdateTemplateByNameAndChannel(name, channel string, updates apiModels.Templatedetails) error {
	var existing apiModels.Templatedetails
	if err := s.DB.Where("TemplateName = ? AND Channel = ?", name, channel).First(&existing).Error; err != nil {
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

// Helper: Convert map to Templatedetails struct
func mapToTemplate(data map[string]interface{}) (*apiModels.Templatedetails, error) {
	template := &apiModels.Templatedetails{
		Id:            int(data["Id"].(int64)),
		TemplateName:  data["TemplateName"].(string),
		ImageId:       data["ImageId"].(string),
		Process:       data["Process"].(string),
		Stage:         int(data["Stage"].(int64)),
		ImageUrl:      data["ImageUrl"].(string),
		DltTemplateId: int64(data["DltTemplateId"].(int64)),
		Channel:       data["Channel"].(string),
		Vendor:        data["Vendor"].(string),
		IsActive: func() bool {
			return data["IsActive"].(int64) == 1
		}(),
		TemplateText: data["TemplateText"].(string),
		Link:         data["Link"].(string),
	}

	if createdOn, ok := data["CreatedOn"].(time.Time); ok {
		template.CreatedOn = createdOn
	}

	if updatedOn, ok := data["UpdatedOn"]; ok && updatedOn != nil {
		switch v := updatedOn.(type) {
		case time.Time:
			template.UpdatedOn = &v
		case string:
			parsed, err := time.Parse("2006-01-02 15:04:05.999 +0000 UTC", v)
			if err != nil {
				return nil, fmt.Errorf("failed to parse UpdatedOn string: %v", err)
			}
			template.UpdatedOn = &parsed
		default:
			return nil, fmt.Errorf("unsupported type for UpdatedOn: %T", updatedOn)
		}
	}

	return template, nil
}
