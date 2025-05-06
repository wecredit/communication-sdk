package services

import (
	"errors"
	"time"

	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
	"gorm.io/gorm"
)

type TemplateService struct {
	DB *gorm.DB
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{DB: db}
}

func (s *TemplateService) GetTemplates(channel, name string) ([]apiModels.Templatedetails, error) {
	var templates []apiModels.Templatedetails
	query := s.DB.Model(&apiModels.Templatedetails{})
	if channel != "" && name == "" {
		query = query.Where("Channel = ?", channel)
	}

	if channel == "" && name != "" {
		query = query.Where("TemplateName = ?", name)
	}

	if channel != "" && name != "" {
		query = query.Where("Channel = ? and TemplateName = ?", channel, name)
	}

	if err := query.Find(&templates).Error; err != nil {
		return nil, err
	}

	return templates, nil
}

func (s *TemplateService) GetTemplateByID(id uint) (*apiModels.Templatedetails, error) {
	var template apiModels.Templatedetails
	if err := s.DB.First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *TemplateService) AddTemplate(template *apiModels.Templatedetails) error {
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	template.CreatedOn = now
	return s.DB.Create(template).Error
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
	return s.DB.Save(&existing).Error
}

func (s *TemplateService) DeleteTemplate(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Templatedetails{})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}
