package services

import (
	"errors"
	"time"

	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
	"gorm.io/gorm"
)

type VendorService struct {
	DB *gorm.DB
}

func NewVendorService(db *gorm.DB) *VendorService {
	return &VendorService{DB: db}
}

func (s *VendorService) GetVendors(channel, name string) ([]apiModels.Vendor, error) {
	var vendors []apiModels.Vendor
	query := s.DB.Model(&apiModels.Vendor{})
	if channel != "" && name == "" {
		query = query.Where("channel = ?", channel)
	}

	if channel == "" && name != "" {
		query = query.Where("name = ?", name)
	}

	if channel != "" && name != "" {
		query = query.Where("channel = ? and name = ?", channel, name)
	}

	if err := query.Find(&vendors).Error; err != nil {
		return nil, err
	}

	return vendors, nil
}

func (s *VendorService) GetVendorByID(id uint) (*apiModels.Vendor, error) {
	var vendor apiModels.Vendor
	if err := s.DB.First(&vendor, id).Error; err != nil {
		return nil, err
	}
	return &vendor, nil
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
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	vendor.CreatedOn = now

	return s.DB.Create(vendor).Error
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
		Select("SUM(weight)").Scan(&sum).Error; err != nil {
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
	return s.DB.Save(&existing).Error
}

func (s *VendorService) DeleteVendor(id int) error {
	result := s.DB.Where("id = ?", id).Delete(&apiModels.Vendor{})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}
