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

type VendorService struct {
	DB *gorm.DB
}

func NewVendorService(db *gorm.DB) *VendorService {
	return &VendorService{DB: db}
}

func (s *VendorService) GetVendors(channel, name, client string) ([]apiModels.Vendor, error) {
	vendorDetails, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return nil, errors.New("vendor data not found in cache")
	}

	channel = strings.ToUpper(strings.TrimSpace(channel))
	name = strings.ToUpper(strings.TrimSpace(name))
	client = strings.ToLower(strings.TrimSpace(client))

	var vendors []apiModels.Vendor

	// Case 1: name, channel and client provided -> direct key lookup
	if name != "" && channel != "" && client != "" {
		key := fmt.Sprintf("Name:%s|Channel:%s|Client:%s", name, channel, client)
		if data, ok := vendorDetails[key]; ok {
			vendor, err := mapToVendor(data)
			if err != nil {
				utils.Error(fmt.Errorf("failed to convert cache data to vendor: %v", err))
				return nil, err
			}
			return []apiModels.Vendor{*vendor}, nil
		}
		return nil, nil // No match found
	}

	// Case 2: loop through entries with applied filters
	for _, data := range vendorDetails {
		dataChannel := strings.ToUpper(extractString(data["Channel"]))
		dataName := strings.ToUpper(extractString(data["Name"]))
		dataClient := strings.ToLower(extractString(data["Client"]))

		if channel != "" && dataChannel != channel {
			continue
		}
		if name != "" && dataName != name {
			continue
		}
		if client != "" && dataClient != client {
			continue
		}
		vendor, err := mapToVendor(data)
		if err != nil {
			utils.Error(fmt.Errorf("skipping invalid vendor data: %v", err))
			continue
		}
		vendors = append(vendors, *vendor)
	}

	return vendors, nil
}

func (s *VendorService) GetVendorByID(id uint) (*apiModels.Vendor, error) {
	// Fetch Id index
	idIndex, found := cache.GetCache().GetMappedIdData(cache.VendorsData + ":IdIndex")
	if !found {
		utils.Error(fmt.Errorf("vendor Id index not found in cache"))
		return nil, errors.New("vendor Id index not found in cache")
	}

	// Look up cache key by Id
	key, ok := idIndex[id]
	if !ok {
		return nil, errors.New("vendor not found")
	}

	// Fetch vendor data using the key
	vendorDetails, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return nil, errors.New("vendor data not found in cache")
	}

	data, ok := vendorDetails[key]
	if !ok {
		return nil, errors.New("vendor not found")
	}

	vendor, err := mapToVendor(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert cache data to vendor: %v", err))
		return nil, err
	}

	return vendor, nil
}

func (s *VendorService) AddVendor(vendor *apiModels.Vendor) error {
	vendor.Name = strings.ToUpper(strings.TrimSpace(vendor.Name))
	vendor.Channel = strings.ToUpper(strings.TrimSpace(vendor.Channel))
	vendor.Client = strings.ToLower(strings.TrimSpace(vendor.Client))

	if vendor.Client == "" {
		return errors.New("client is required")
	}

	var totalWeight int64

	err := s.DB.Model(apiModels.Vendor{}).
		Where("channel = ? AND client = ? AND status = 1", vendor.Channel, vendor.Client).
		Select("COALESCE(SUM(weight), 0)").
		Scan(&totalWeight).Error
	if err != nil {
		return err
	}

	if totalWeight+int64(vendor.Weight) > 100 {
		return errors.New("weight exceeds 100 for active vendors on this channel")
	}

	vendor.Status = 1
	vendor.IsHealthy = 1
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	vendor.CreatedOn = now

	err = s.DB.Create(vendor).Error
	if err != nil {
		return err
	}
	cache.StoreMappedDataIntoCache(cache.VendorsData, config.Configs.VendorTable, "Name", "Channel", s.DB)
	return nil
}

func (s *VendorService) UpdateVendorByNameAndChannel(_, _ string, updates apiModels.Vendor) error {
	updates.Name = strings.ToUpper(strings.TrimSpace(updates.Name))
	updates.Channel = strings.ToUpper(strings.TrimSpace(updates.Channel))
	updates.Client = strings.ToLower(strings.TrimSpace(updates.Client))

	if updates.Client == "" {
		return errors.New("client is required")
	}

	var existing apiModels.Vendor
	if err := s.DB.Where("name = ? AND channel = ? AND client = ?", updates.Name, updates.Channel, updates.Client).First(&existing).Error; err != nil {
		return errors.New("vendor not found")
	}

	// Calculate total weight excluding this vendor
	var sum int64
	if err := s.DB.Model(&apiModels.Vendor{}).
		Where("channel = ? AND client = ? AND status = 1 AND name != ?", updates.Channel, updates.Client, updates.Name).
		Select("COALESCE(SUM(weight), 0)").Scan(&sum).Error; err != nil {
		return err
	}

	if updates.Status == 1 && sum+int64(updates.Weight) > 100 {
		return errors.New("updated weight exceeds 100 for active vendors on this channel")
	}

	existing.Status = updates.Status
	existing.IsHealthy = updates.IsHealthy
	existing.Weight = updates.Weight
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	existing.UpdatedOn = &now
	err := s.DB.Save(&existing).Error
	if err != nil {
		return err
	}
	cache.StoreMappedDataIntoCache(cache.VendorsData, config.Configs.VendorTable, "Name", "Channel", s.DB)
	return nil
}

func (s *VendorService) DeleteVendor(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Vendor{})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	cache.StoreMappedDataIntoCache(cache.VendorsData, config.Configs.VendorTable, "Name", "Channel", s.DB)

	return nil
}

// Helper function to convert map to Vendor struct
func mapToVendor(data map[string]interface{}) (*apiModels.Vendor, error) {
	vendor := &apiModels.Vendor{
		Id:        int(data["Id"].(int64)),
		Name:      extractString(data["Name"]),
		Channel:   extractString(data["Channel"]),
		Client:    strings.ToLower(extractString(data["Client"])),
		Status:    int(data["Status"].(int64)),
		IsHealthy: int(data["IsHealthy"].(int64)),
		Weight:    int(data["Weight"].(int64)),
	}

	// Parse CreatedOn
	if createdOn, ok := data["CreatedOn"].(time.Time); ok {
		// parsed, err := time.Parse("2006-01-02 15:04:05.999 +0000 UTC", createdOn)
		// if err != nil {
		// 	return nil, fmt.Errorf("failed to parse CreatedOn: %v", err)
		// }
		vendor.CreatedOn = createdOn
	}

	// Parse UpdatedOn (nullable)
	if updatedOn, ok := data["UpdatedOn"]; ok && updatedOn != nil {
		switch v := updatedOn.(type) {
		case time.Time:
			vendor.UpdatedOn = &v
		case string:
			parsed, err := time.Parse("2006-01-02 15:04:05.999 +0000 UTC", v)
			if err != nil {
				return nil, fmt.Errorf("failed to parse UpdatedOn string: %v", err)
			}
			vendor.UpdatedOn = &parsed
		default:
			return nil, fmt.Errorf("unsupported type for UpdatedOn: %T", updatedOn)
		}
	}

	return vendor, nil
}

func extractString(val interface{}) string {
	switch v := val.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
