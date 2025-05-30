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

type ClientService struct {
	DB *gorm.DB
}

func NewClientService(db *gorm.DB) *ClientService {
	return &ClientService{DB: db}
}

func (s *ClientService) GetClients(channel, name string) ([]apiModels.Client, error) {
	clientDetails, found := cache.GetCache().GetMappedData(cache.ClientsData)
	if !found {
		utils.Error(fmt.Errorf("client data not found in cache"))
		return nil, errors.New("client data not found in cache")
	}

	var clients []apiModels.Client

	// Case 1: Both name and channel provided -> direct key lookup
	if name != "" && channel != "" {
		key := fmt.Sprintf("Name:%s|Channel:%s", name, channel)
		if data, ok := clientDetails[key]; ok {
			client, err := mapToClient(data)
			if err != nil {
				utils.Error(fmt.Errorf("failed to convert cache data to client: %v", err))
				return nil, err
			}
			return []apiModels.Client{*client}, nil
		}
		return nil, nil // No match
	}

	// Case 2: Filtering loop
	for _, data := range clientDetails {
		if (channel != "" && data["Channel"] != channel) || (name != "" && data["Name"] != name) {
			continue
		}
		client, err := mapToClient(data)
		if err != nil {
			utils.Error(fmt.Errorf("skipping invalid client data: %v", err))
			continue
		}
		clients = append(clients, *client)
	}

	return clients, nil
}

func (s *ClientService) GetClientByID(id uint) (*apiModels.Client, error) {
	idIndex, found := cache.GetCache().GetMappedIdData(cache.ClientsData + ":IdIndex")
	if !found {
		utils.Error(fmt.Errorf("client Id index not found in cache"))
		return nil, errors.New("client Id index not found in cache")
	}

	key, ok := idIndex[id]
	if !ok {
		return nil, errors.New("client not found")
	}

	clientDetails, found := cache.GetCache().GetMappedData(cache.ClientsData)
	if !found {
		utils.Error(fmt.Errorf("client data not found in cache"))
		return nil, errors.New("client data not found in cache")
	}

	data, ok := clientDetails[key]
	if !ok {
		return nil, errors.New("client not found")
	}

	client, err := mapToClient(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert cache data to client: %v", err))
		return nil, err
	}

	return client, nil
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
		return errors.New("add credentials for this client first")
	}

	client.Name = strings.ToLower(client.Name)
	client.Channel = strings.ToUpper(client.Channel)

	client.Status = 1
	istOffset := 5*time.Hour + 30*time.Minute
	client.CreatedOn = time.Now().UTC().Add(istOffset)

	err = s.DB.Create(client).Error
	if err != nil {
		return err
	}
	cache.StoreMappedDataIntoCache(cache.ClientsData, config.Configs.ClientsTable, "Name", "Channel", s.DB)
	return nil
}

func (s *ClientService) UpdateClientByNameAndChannel(name, channel string, updates apiModels.Client) error {
	var existing apiModels.Client
	if err := s.DB.Where("name = ? AND channel = ?", name, channel).First(&existing).Error; err != nil {
		return errors.New("client not found")
	}

	existing.Name = strings.ToLower(existing.Name)
	existing.Channel = strings.ToUpper(existing.Channel)
	existing.Status = updates.Status
	existing.RateLimitPerMinute = updates.RateLimitPerMinute
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	existing.UpdatedOn = &now

	err := s.DB.Save(&existing).Error
	if err != nil {
		return err
	}
	cache.StoreMappedDataIntoCache(cache.ClientsData, config.Configs.ClientsTable, "Name", "Channel", s.DB)
	return nil
}

func (s *ClientService) DeleteClient(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Client{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	cache.StoreMappedDataIntoCache(cache.ClientsData, config.Configs.ClientsTable, "Name", "Channel", s.DB)
	return nil
}

func (s *ClientService) ValidateCredentials(username, password string) (*apiModels.Userbasicauth, error) {
	var user apiModels.Userbasicauth

	// Collecting BasicAuthData
	authDetails, _ := cache.GetCache().Get("authDetails")

	// Validate the credentials
	isValid := false
	for _, data := range authDetails {
		// extract username and password from the map
		usernameFromData, _ := data["username"].(string)
		passwordFromData, _ := data["password"].(string)

		// Validate headers username and password
		if usernameFromData == username && passwordFromData == password {
			isValid = true
			break
		}

	}

	if !isValid {
		return nil, errors.New("invalid Username and Password")
	}

	// err := s.DB.Where("username = ? AND password = ?", username, password).First(&user).Error
	// if err != nil {
	// 	if errors.Is(err, gorm.ErrRecordNotFound) {
	// 		return nil, gorm.ErrRecordNotFound
	// 	}
	// 	return nil, err
	// }
	return &user, nil
}

// Helper function to convert map to Client struct
func mapToClient(data map[string]interface{}) (*apiModels.Client, error) {
	client := &apiModels.Client{
		Id:                 int(data["Id"].(int64)),
		Name:               data["Name"].(string),
		Channel:            data["Channel"].(string),
		Status:             int(data["Status"].(int64)),
		RateLimitPerMinute: int(data["RateLimitPerMinute"].(int64)),
	}

	if createdOn, ok := data["CreatedOn"].(time.Time); ok {
		client.CreatedOn = createdOn
	}

	if updatedOn, ok := data["UpdatedOn"]; ok && updatedOn != nil {
		switch v := updatedOn.(type) {
		case time.Time:
			client.UpdatedOn = &v
		case string:
			parsed, err := time.Parse("2006-01-02 15:04:05.999 +0000 UTC", v)
			if err != nil {
				return nil, fmt.Errorf("failed to parse UpdatedOn string: %v", err)
			}
			client.UpdatedOn = &parsed
		default:
			return nil, fmt.Errorf("unsupported type for UpdatedOn: %T", updatedOn)
		}
	}

	return client, nil
}
