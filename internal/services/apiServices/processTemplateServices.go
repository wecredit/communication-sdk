package apiServices

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
	"gorm.io/gorm"
)

type TemplateService struct {
	DB *gorm.DB
}

func NewTemplateService(db *gorm.DB) *TemplateService {
	return &TemplateService{DB: db}
}

func (s *TemplateService) GetTemplates(process, stage, client, channel, vendor string) ([]apiModels.Templatedetails, error) {
	// cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB) // Temporary: ensure cache is populated
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
		var stageFloat float64
		if stage != "" {
			val, ok := data["Stage"].(float64)
			if !ok {
				return nil, fmt.Errorf("stage unexpected type=%T value=%v", data["Stage"], data["Stage"])
			}
			stageFloat = val
		}

		if (process != "" && data["Process"] != process) ||
			(stage != "" && fmt.Sprintf("%.2f", stageFloat) != stage) ||
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
		if templates[i].Stage != templates[j].Stage {
			return templates[i].Stage < templates[j].Stage
		}
		return templates[i].Vendor < templates[j].Vendor
	})

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
	if s.DB == nil {
		return errors.New("database connection not initialized")
	}

	// Trim and validate required fields
	template.Channel = strings.TrimSpace(template.Channel)
	if template.Channel == "" {
		return errors.New("channel cannot be empty or whitespace")
	}

	template.Vendor = strings.TrimSpace(template.Vendor)
	if template.Vendor == "" {
		return errors.New("vendor cannot be empty or whitespace")
	}

	// Normalize for GetRequiredFields call (before uppercase conversion)
	vendorUpper := strings.ToUpper(strings.TrimSpace(template.Vendor))
	channelUpper := strings.ToUpper(strings.TrimSpace(template.Channel))

	// Get required fields based on vendor and channel
	reqFieldsResp, err := s.GetRequiredFields(vendorUpper, channelUpper)
	if err != nil {
		// This handles unsupported vendor/channel combinations
		return err
	}

	// Validate all required fields are present
	missingFields := validateTemplateRequiredFields(template, reqFieldsResp.RequiredFields)
	if len(missingFields) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missingFields, ", "))
	}

	// Process is optional, but trim if provided
	template.Process = strings.TrimSpace(template.Process)

	// Client is optional, but trim if provided
	template.Client = strings.TrimSpace(template.Client)
	istOffset := 5*time.Hour + 30*time.Minute
	template.CreatedOn = time.Now().UTC().Add(istOffset)
	template.Process = strings.ToUpper(template.Process)
	template.Channel = channelUpper
	template.Vendor = vendorUpper
	template.Client = strings.ToLower(template.Client)

	// Check for active template conflict before creating
	if template.IsActive {
		templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
		if !found {
			return fmt.Errorf("template data not found in cache for conflict check")
		}

		// Try direct key lookup first (faster than full scan)
		stageKey := fmt.Sprintf("%.2f", template.Stage)
		lookupKey := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s",
			template.Process, stageKey, template.Channel, template.Vendor)
		checkedDirect := false
		if data, ok := templateDetails[lookupKey]; ok {
			checkedDirect = true
			existing, err := mapToTemplate(data)
			if err == nil && existing.IsActive {
				return fmt.Errorf("another active template (ID: %d) already exists for this process-stage-vendor-channel combination", existing.Id)
			}
		}

		// Fallback to scan cache for any active match only if direct lookup missed or failed
		if !checkedDirect {
			for _, data := range templateDetails {
				existing, err := mapToTemplate(data)
				if err != nil {
					utils.Error(fmt.Errorf("skipping invalid template data during conflict check: %v", err))
					continue
				}
				if existing.IsActive &&
					existing.Process == template.Process &&
					existing.Stage == template.Stage &&
					existing.Vendor == template.Vendor &&
					existing.Channel == template.Channel {
					return fmt.Errorf("another active template (ID: %d) already exists for this process-stage-vendor-channel combination", existing.Id)
				}
			}
		}
	}

	err = s.DB.Create(template).Error
	if err != nil {
		return err
	}

	cache.StoreMappedDataIntoCache(cache.TemplateDetailsData, config.Configs.TemplateDetailsTable, "Process", "Stage", s.DB)

	return nil
}

func (s *TemplateService) UpdateTemplateById(id int, updates map[string]interface{}) error {
	var existing apiModels.Templatedetails
	if err := s.DB.Where("id = ?", id).First(&existing).Error; err != nil {
		return errors.New("template not found")
	}

	// Validate and sanitize fields if present in updates
	if channel, ok := updates["channel"].(string); ok {
		channel = strings.TrimSpace(channel)
		if channel == "" {
			return errors.New("channel cannot be empty or whitespace")
		}
		updates["channel"] = strings.ToUpper(channel)
	}

	if vendor, ok := updates["vendor"].(string); ok {
		vendor = strings.TrimSpace(vendor)
		if vendor == "" {
			return errors.New("vendor cannot be empty or whitespace")
		}
		updates["vendor"] = strings.ToUpper(vendor)
	}

	if process, ok := updates["process"].(string); ok {
		updates["process"] = strings.ToUpper(strings.TrimSpace(process))
	}

	if client, ok := updates["client"].(string); ok {
		updates["client"] = strings.ToLower(strings.TrimSpace(client))
	}

	if stage, ok := updates["stage"].(float64); ok {
		if stage <= 0 {
			return errors.New("stage must be greater than 0")
		}
	}

	// Check for active template conflict before updating (cache based, excluding current ID)
	if isActive, ok := updates["isActive"].(bool); ok && isActive {
		templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
		if !found {
			return fmt.Errorf("template data not found in cache for conflict check")
		}

		// Direct key lookup first (faster)
		stageKey := fmt.Sprintf("%.2f", existing.Stage)
		lookupKey := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s",
			existing.Process, stageKey, existing.Channel, existing.Vendor)
		checkedDirect := false
		if data, ok := templateDetails[lookupKey]; ok {
			checkedDirect = true
			tmpl, err := mapToTemplate(data)
			if err == nil && tmpl.IsActive && tmpl.Id != id {
				return fmt.Errorf("another active template (ID: %d) already exists for this process-stage-vendor-channel combination", tmpl.Id)
			}
		}

		// Fallback to scan cache if direct lookup missed
		if !checkedDirect {
			for _, data := range templateDetails {
				tmpl, err := mapToTemplate(data)
				if err != nil {
					utils.Error(fmt.Errorf("skipping invalid template data during conflict check: %v", err))
					continue
				}
				if tmpl.IsActive &&
					tmpl.Id != id &&
					tmpl.Process == existing.Process &&
					tmpl.Stage == existing.Stage &&
					tmpl.Vendor == existing.Vendor &&
					tmpl.Channel == existing.Channel {
					return fmt.Errorf("another active template (ID: %d) already exists for this process-stage-vendor-channel combination", tmpl.Id)
				}
			}
		}
	}

	// add updatedOn timestamp
	istOffset := 5*time.Hour + 30*time.Minute
	updates["UpdatedOn"] = time.Now().UTC().Add(istOffset)

	if err := s.DB.Model(&existing).Updates(updates).Error; err != nil {
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

	getBool := func(key string) bool {
		if val, ok := data[key].(int64); ok {
			return val == 1
		}
		return false
	}

	template := &apiModels.Templatedetails{
		Id:                getInt("Id"),
		Client:            getStr("Client"),
		Channel:           getStr("Channel"),
		Process:           getStr("Process"),
		Stage:             getFloat("Stage"),
		Vendor:            getStr("Vendor"),
		TemplateName:      getStr("TemplateName"),
		ImageId:           getStr("ImageId"),
		ImageUrl:          getStr("ImageUrl"),
		DltTemplateId:     int64(getInt("DltTemplateId")), // stored as int64 anyway
		IsActive:          getBool("IsActive"),
		TemplateText:      getStr("TemplateText"),
		TemplateCategory:  int64(getInt("TemplateCategory")),
		TemplateVariables: getStr("TemplateVariables"),
		FromEmail:         getStr("FromEmail"),
		Subject:           getStr("Subject"),
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

// GetRequiredFields returns required fields based on vendor and channel combination
func (s *TemplateService) GetRequiredFields(vendor, channel string) (*apiModels.RequiredFieldsResponse, error) {
	vendor = strings.ToUpper(strings.TrimSpace(vendor))
	channel = strings.ToUpper(strings.TrimSpace(channel))

	if vendor == "" {
		return nil, errors.New(variables.ErrVendorRequired)
	}
	if channel == "" {
		return nil, errors.New(variables.ErrChannelRequired)
	}

	var requiredFields []string

	switch channel {
	case "WHATSAPP":
		switch vendor {
		case "SINCH":
			requiredFields = []string{"templateName", "imageId", "link", "templateVariables", "process", "stage", "templatetext", "client", "isActive"}
		case "TIMES":
			requiredFields = []string{"templateName", "imageUrl", "link", "process", "stage", "templatetext", "client", "isActive"}
		default:
			return nil, fmt.Errorf("unsupported vendor '%s' for WhatsApp channel", vendor)
		}

	case "SMS":
		switch vendor {
		case "SINCH":
			requiredFields = []string{"templateName", "templateText", "dltTemplateId", "templateVariables", "templateCategory", "process", "stage", "client", "isActive"}
		case "TIMES":
			requiredFields = []string{"templateName", "templateText", "dltTemplateId", "process", "stage", "client", "isActive"}
		default:
			return nil, fmt.Errorf("unsupported vendor '%s' for SMS channel", vendor)
		}

	case "EMAIL":
		// Email requirements are same for both SINCH and TIMES vendors
		switch vendor {
		case "SINCH", "TIMES":
			requiredFields = []string{"templateName", "subject", "fromEmail", "templateText", "process", "stage", "client", "Link"}
		default:
			return nil, fmt.Errorf("unsupported vendor '%s' for EMAIL channel", vendor)
		}

	default:
		return nil, fmt.Errorf("unsupported channel: %s", channel)
	}

	return &apiModels.RequiredFieldsResponse{
		Vendor:         vendor,
		Channel:        channel,
		RequiredFields: requiredFields,
	}, nil
}

// validateTemplateRequiredFields validates that all required fields are present and non-empty
// Returns a list of missing field names in camelCase format
func validateTemplateRequiredFields(template *apiModels.Templatedetails, requiredFields []string) []string {
	var missingFields []string

	for _, field := range requiredFields {
		fieldLower := strings.ToLower(field)
		var isEmpty bool
		var displayName string

		switch fieldLower {
		case "templatename":
			isEmpty, displayName = template.TemplateName == "", "templateName"
		case "imageid":
			isEmpty, displayName = template.ImageId == "", "imageId"
		case "imageurl":
			isEmpty, displayName = template.ImageUrl == "", "imageUrl"
		case "link":
			isEmpty, displayName = template.Link == "", "link"
		case "templatevariables":
			isEmpty, displayName = template.TemplateVariables == "", "templateVariables"
		case "process":
			isEmpty, displayName = template.Process == "", "process"
		case "stage":
			isEmpty, displayName = template.Stage == 0, "stage"
		case "templatetext": // Handles both "templatetext" and "templateText"
			isEmpty, displayName = template.TemplateText == "", "templateText"
		case "client":
			isEmpty, displayName = template.Client == "", "client"
		case "dlttemplateid":
			isEmpty, displayName = template.DltTemplateId == 0, "dltTemplateId"
		case "templatecategory":
			isEmpty, displayName = template.TemplateCategory == 0, "templateCategory"
		case "subject":
			isEmpty, displayName = template.Subject == "", "subject"
		case "fromemail":
			isEmpty, displayName = template.FromEmail == "", "fromEmail"
		default:
			continue // Skip unknown fields
		}

		if isEmpty {
			missingFields = append(missingFields, displayName)
		}
	}

	return missingFields
}
