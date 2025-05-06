package services

import (
	"errors"
	"time"

	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
	"gorm.io/gorm"
)

type ClientService struct {
	DB *gorm.DB
}

func NewClientService(db *gorm.DB) *ClientService {
	return &ClientService{DB: db}
}

func (s *ClientService) GetClients(channel, name string) ([]apiModels.Client, error) {
	var clients []apiModels.Client
	query := s.DB.Model(&apiModels.Client{})
	if channel != "" && name == "" {
		query = query.Where("channel = ?", channel)
	}

	if channel == "" && name != "" {
		query = query.Where("name = ?", name)
	}

	if channel != "" && name != "" {
		query = query.Where("channel = ? and name = ?", channel, name)
	}

	if err := query.Find(&clients).Error; err != nil {
		return nil, err
	}

	return clients, nil
}

func (s *ClientService) GetClientByID(id uint) (*apiModels.Client, error) {
	var client apiModels.Client
	if err := s.DB.First(&client, id).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *ClientService) AddClient(client *apiModels.Client) error {
	var clientName string
	err := s.DB.Model(apiModels.Userbasicauth{}).
		Where("username = ?", client.Name).
		Select("username").
		Scan(&clientName).Error
	if err != nil {
		return err
	}

	if clientName == "" {
		return errors.New("add Credentials for this Client first")
	}

	client.Status = 1
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	client.CreatedOn = now

	return s.DB.Create(client).Error
}

func (s *ClientService) UpdateClientByNameAndChannel(name, channel string, updates apiModels.Client) error {
	var existing apiModels.Client
	if err := s.DB.Where("name = ? AND channel = ?", name, channel).First(&existing).Error; err != nil {
		return errors.New("client not found")
	}

	existing.Status = updates.Status
	existing.RateLimitPerMinute = updates.RateLimitPerMinute
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	existing.UpdatedOn = &now
	return s.DB.Save(&existing).Error
}

func (s *ClientService) DeleteClient(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Client{})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func (s *ClientService) ValidateCredentials(username, password string) (*apiModels.Userbasicauth, error) {
	var user apiModels.Userbasicauth

	err := s.DB.
		Where("username = ? AND password = ?", username, password).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}

	return &user, nil
}
