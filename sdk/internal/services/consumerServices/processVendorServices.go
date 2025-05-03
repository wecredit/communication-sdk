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

func (s *VendorService) AddVendor(v *apiModels.Vendor) error {
	var totalWeight int64

	err := s.DB.Model(apiModels.Vendor{}).
		Where("channel = ? AND status = 1", v.Channel).
		Select("COALESCE(SUM(weight), 0)").
		Scan(&totalWeight).Error
	if err != nil {
		return err
	}

	if totalWeight+int64(v.Weight) > 100 {
		return errors.New("weight exceeds 100 for active vendors on this channel")
	}

	v.Status = 1
	v.IsHealthy = 1
	v.CreatedOn = time.Now()

	return s.DB.Create(v).Error
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

	existing.Name = updates.Name
	existing.Channel = updates.Channel
	existing.Status = updates.Status
	existing.IsHealthy = updates.IsHealthy
	existing.Weight = updates.Weight
	istOffset := 5*time.Hour + 30*time.Minute
	now := time.Now().UTC().Add(istOffset)
	existing.UpdatedOn = &now
	return s.DB.Save(&existing).Error
}


func (s *VendorService) DeleteVendor(id uint) error {
	return s.DB.Delete(&apiModels.Vendor{}, id).Error
}

func (s *VendorService) GetVendorByID(id uint) (*apiModels.Vendor, error) {
	var vendor apiModels.Vendor
	if err := s.DB.First(&vendor, id).Error; err != nil {
		return nil, err
	}
	return &vendor, nil
}

func (s *VendorService) GetVendors(channel string) ([]apiModels.Vendor, error) {
	var vendors []apiModels.Vendor
	query := s.DB.Model(&apiModels.Vendor{})
	if channel != "" {
		query = query.Where("channel = ?", channel)
	}
	if err := query.Find(&vendors).Error; err != nil {
		return nil, err
	}
	return vendors, nil
}
