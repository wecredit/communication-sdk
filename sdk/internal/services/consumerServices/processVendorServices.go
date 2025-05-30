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

type VendorService struct {
	DB *gorm.DB
}

func NewVendorService(db *gorm.DB) *VendorService {
	return &VendorService{DB: db}
}

func (s *VendorService) GetVendors(channel, name string) ([]apiModels.Vendor, error) {
	vendorDetails, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return nil, errors.New("vendor data not found in cache")
	}

	var vendors []apiModels.Vendor

	// Case 1: Both name and channel provided -> direct key lookup
	if name != "" && channel != "" {
		key := fmt.Sprintf("Name:%s|Channel:%s", name, channel)
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

	// Case 2: Only channel or only name provided, or no filters -> loop through entries
	for _, data := range vendorDetails {
		if (channel != "" && data["Channel"] != channel) || (name != "" && data["Name"] != name) {
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
	var totalWeight int64

	err := s.DB.Model(apiModels.Vendor{}).
		Where("channel = ? AND status = 1", vendor.Channel).
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
	vendor.Name = strings.ToUpper(vendor.Name)
	vendor.Channel = strings.ToUpper(vendor.Channel)
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

func (s *VendorService) UpdateVendorByNameAndChannel(name, channel string, updates apiModels.Vendor) error {
	var existing apiModels.Vendor
	if err := s.DB.Where("name = ? AND channel = ?", name, channel).First(&existing).Error; err != nil {
		return errors.New("vendor not found")
	}

	// Calculate total weight excluding this vendor
	var sum int64
	if err := s.DB.Model(&apiModels.Vendor{}).
		Where("channel = ? AND status = 1 AND name != ?", channel, name).
		Select("COALESCE(SUM(weight), 0)").Scan(&sum).Error; err != nil {
		return err
	}

	if updates.Status == 1 && sum+int64(updates.Weight) > 100 {
		return errors.New("updated weight exceeds 100 for active vendors on this channel")
	}

	existing.Status = updates.Status
	existing.IsHealthy = updates.IsHealthy
	existing.Weight = updates.Weight
	existing.Name = strings.ToUpper(existing.Name)
	existing.Channel = strings.ToUpper(existing.Channel)
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
		Name:      data["Name"].(string),
		Channel:   data["Channel"].(string),
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
