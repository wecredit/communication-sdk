package channelHelper

import (
	"fmt"
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func ConstructTemplateKey(msg sdkModels.CommApiRequestBody) string {
	return fmt.Sprintf("Process:%s|Stage:%.2f|Client:%s|Channel:%s|Vendor:%s",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor)
}

// FetchTemplateData attempts exact key match, falls back to wildcard vendor if needed.
func FetchTemplateData(msg sdkModels.CommApiRequestBody, templateDetails map[string]map[string]interface{}) (map[string]interface{}, string, error) {
	key := ConstructTemplateKey(msg)
	if data, ok := templateDetails[key]; ok {
		return data, msg.Vendor, nil
	}

	if msg.Client == variables.CreditSea {
		return nil, msg.Vendor, fmt.Errorf("no template found for Process: %s, Stage: %.2f, Client: %s, Channel: %s, Vendor: %s",
			msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor)
	}

	// No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s, Vendor: %s; Fetching fallback template
	utils.Debug(fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s, Vendor: %s; Fetching fallback template",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor))
	prefix := fmt.Sprintf("Process:%s|Stage:%.2f|Client:%s|Channel:%s|Vendor:",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel)
	for otherKey, val := range templateDetails {
		if strings.HasPrefix(otherKey, prefix) {
			parts := strings.Split(otherKey, "|")
			if len(parts) == 5 {
				vendor := strings.TrimPrefix(parts[4], "Vendor:")
				if IsVendorActive(vendor, msg.Channel) {
					return val, vendor, nil
				}
			}
		}
	}

	return nil, "", fmt.Errorf("no fallback template found for Process: %s, Stage: %.2f, Client: %s, Channel: %s",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel)
}

func IsVendorActive(vendor, channel string) bool {
	vendors, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return false
	}
	key := fmt.Sprintf("Name:%s|Channel:%s", vendor, channel)
	if vendorData, ok := vendors[key]; ok {
		if status, ok := vendorData["Status"].(int64); ok && status == variables.Active {
			return true
		}
	}
	return false
}

func LogTemplateNotFound(msg sdkModels.CommApiRequestBody, err error) {
	utils.Error(fmt.Errorf("template missing for CommId %s: %v", msg.CommId, err))
}

func PopulateWhatsappFields(req *extapimodels.WhatsappRequestBody, data map[string]interface{}) {
	if val, ok := data["TemplateName"].(string); ok {
		req.TemplateName = val
	}
	if val, ok := data["ImageUrl"].(string); ok {
		req.ImageUrl = val
	}
	if val, ok := data["ImageId"].(string); ok {
		req.ImageID = val
	}
	if val, ok := data["Link"].(string); ok {
		req.ButtonLink = val
	}
	if val, ok := data["TemplateVariables"].(string); ok {
		req.TemplateVariables = val
	}
	if val, ok := data["TemplateCategory"].(int64); ok {
		req.TemplateCategory = fmt.Sprintf("%d", val)
	}
}

func PopulateSmsFields(req *extapimodels.SmsRequestBody, data map[string]interface{}) {
	if val, ok := data["TemplateText"].(string); ok {
		req.TemplateText = val
	}
	if val, ok := data["TemplateVariables"].(string); ok {
		req.TemplateVariables = val
	}
	if val, ok := data["DltTemplateId"].(int64); ok {
		req.DltTemplateId = val
	}
	if val, ok := data["TemplateCategory"].(int64); ok {
		req.TemplateCategory = fmt.Sprintf("%d", val)
	}
}

func PopulateRcsFields(req *extapimodels.RcsRequestBody, data map[string]interface{}) {
	if val, ok := data["TemplateName"].(string); ok {
		req.TemplateName = val
	}
	if val, ok := data["ImageId"].(string); ok {
		req.AppId = val
	}
}

func PopulateEmailFields(req *extapimodels.EmailRequestBody, data map[string]interface{}) {
	if val, ok := data["TemplateName"].(string); ok {
		req.TemplateId = val
	}
	if val, ok := data["Subject"].(string); ok {
		req.EmailSubject = val
	}
	if val, ok := data["TemplateVariables"].(string); ok {
		req.TemplateVariables = val
	}
	if val, ok := data["FromEmail"].(string); ok {
		req.FromEmail = val
	}
}
