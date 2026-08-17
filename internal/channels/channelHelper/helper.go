package channelHelper

import (
	"fmt"
	"strconv"
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

// ResolveTemplateData uses TemplateReference lookup for CommMarketingInput dispatch rows;
// legacy nurture journeys continue to resolve by Process+Stage.
func ResolveTemplateData(msg sdkModels.CommApiRequestBody, templateDetails map[string]map[string]interface{}) (map[string]interface{}, string, error) {
	if strings.TrimSpace(msg.TemplateReference) != "" {
		return FetchTemplateDataByReference(msg, templateDetails)
	}
	return FetchTemplateData(msg, templateDetails)
}

// FetchTemplateDataByReference resolves an active template for CommMarketingInput dispatch.
// SMS uses DltTemplateId; RCS/WhatsApp/Email use TemplateName.
func FetchTemplateDataByReference(msg sdkModels.CommApiRequestBody, templateDetails map[string]map[string]interface{}) (map[string]interface{}, string, error) {
	templateRef := strings.TrimSpace(msg.TemplateReference)
	if templateRef == "" {
		return nil, msg.Vendor, fmt.Errorf("template reference is empty")
	}

	client := strings.ToLower(strings.TrimSpace(msg.Client))
	channel := strings.ToUpper(strings.TrimSpace(msg.Channel))
	vendor := strings.ToUpper(strings.TrimSpace(msg.Vendor))
	process := strings.TrimSpace(msg.ProcessName)

	var matches []map[string]interface{}
	for _, val := range templateDetails {
		if val["IsActive"] != variables.Active {
			continue
		}
		if channel == "SMS" {
			if !templateMatchesSMSReference(val, templateRef) {
				continue
			}
		} else {
			name, _ := val["TemplateName"].(string)
			if !strings.EqualFold(strings.TrimSpace(name), templateRef) {
				continue
			}
		}
		rowClient, _ := val["Client"].(string)
		rowChannel, _ := val["Channel"].(string)
		rowVendor, _ := val["Vendor"].(string)
		rowProcess, _ := val["Process"].(string)
		if !strings.EqualFold(strings.TrimSpace(rowClient), client) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rowChannel), channel) {
			continue
		}
		if vendor != "" && !strings.EqualFold(strings.TrimSpace(rowVendor), vendor) {
			continue
		}
		if process != "" && !strings.EqualFold(strings.TrimSpace(rowProcess), process) {
			continue
		}
		matches = append(matches, val)
	}

	switch len(matches) {
	case 0:
		return nil, vendor, fmt.Errorf("no template found for reference: %s, Process: %s, Client: %s, Channel: %s, Vendor: %s",
			templateRef, process, client, channel, vendor)
	case 1:
		resolvedVendor := vendor
		if resolvedVendor == "" {
			if v, ok := matches[0]["Vendor"].(string); ok {
				resolvedVendor = strings.ToUpper(strings.TrimSpace(v))
			}
		}
		return matches[0], resolvedVendor, nil
	default:
		return nil, vendor, fmt.Errorf("multiple active templates found for reference: %s, Process: %s, Client: %s, Channel: %s, Vendor: %s",
			templateRef, process, client, channel, vendor)
	}
}

func templateMatchesSMSReference(row map[string]interface{}, templateRef string) bool {
	switch v := row["DltTemplateId"].(type) {
	case int64:
		return strconv.FormatInt(v, 10) == templateRef
	case int:
		return strconv.Itoa(v) == templateRef
	case float64:
		return strconv.FormatInt(int64(v), 10) == templateRef
	case string:
		return strings.TrimSpace(v) == templateRef
	default:
		return false
	}
}

// FetchTemplateData attempts exact key match, then same-vendor stage fallback.
// When the request already has an explicit vendor, cross-vendor fallback is disabled
// so AssignVendor's selection cannot be overwritten by another active template.
func FetchTemplateData(msg sdkModels.CommApiRequestBody, templateDetails map[string]map[string]interface{}) (map[string]interface{}, string, error) {
	key := ConstructTemplateKey(msg)
	if data, ok := templateDetails[key]; ok && data["IsActive"] == variables.Active { // Check if template is active  //TODO: Check if any stage has an active and inactive template then active gets sent.
		return data, msg.Vendor, nil
	}

	// 2. Stage fallback (same Process, same Client, same Channel, same Vendor, but any sub-stage in same integer part)
	stageInt := int(msg.Stage)
	stagePrefix := fmt.Sprintf("Process:%s|Stage:%d.", msg.ProcessName, stageInt)

	utils.Debug(fmt.Sprintf(
		"No exact template found for Stage %.2f; Trying stage fallback within stage %d for vendor %s, channel %s...",
		msg.Stage, stageInt, msg.Vendor, msg.Channel))

	for otherKey, val := range templateDetails {
		if strings.HasPrefix(otherKey, stagePrefix) && val["IsActive"] == variables.Active {
			parts := strings.Split(otherKey, "|")
			if len(parts) == 5 &&
				parts[2] == "Client:"+msg.Client &&
				parts[3] == "Channel:"+msg.Channel &&
				parts[4] == "Vendor:"+msg.Vendor {
				return val, msg.Vendor, nil
			}
		}
	}

	explicitVendor := strings.TrimSpace(msg.Vendor)
	// Explicit vendor (including CreditSea): fail closed — never switch to another vendor's template.
	if explicitVendor != "" || msg.Client == variables.CreditSea {
		return nil, msg.Vendor, fmt.Errorf("no template found for Process: %s, Stage: %.2f, Client: %s, Channel: %s, Vendor: %s",
			msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor)
	}

	// Legacy path only: empty vendor may resolve via any active vendor template for the same process/stage/channel.
	utils.Debug(fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s; Fetching cross-vendor fallback template",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel))
	prefix := fmt.Sprintf("Process:%s|Stage:%.2f|Client:%s|Channel:%s|Vendor:",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel)
	for otherKey, val := range templateDetails {
		if strings.HasPrefix(otherKey, prefix) && val["IsActive"] == variables.Active {
			parts := strings.Split(otherKey, "|")
			if len(parts) == 5 {
				vendor := strings.TrimPrefix(parts[4], "Vendor:")
				if IsVendorActive(msg.Client, vendor, msg.Channel) {
					return val, vendor, nil
				}
			}
		}
	}

	return nil, "", fmt.Errorf("no fallback template found for Process: %s, Stage: %.2f, Client: %s, Channel: %s",
		msg.ProcessName, msg.Stage, msg.Client, msg.Channel)
}

func IsVendorActive(client, vendor, channel string) bool {
	vendors, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return false
	}

	clientKey := strings.ToLower(strings.TrimSpace(client))
	channelKey := strings.ToUpper(strings.TrimSpace(channel))
	baseKey := fmt.Sprintf("Name:%s|Channel:%s", vendor, channelKey)

	keysToTry := []string{
		fmt.Sprintf("%s|Client:%s", baseKey, clientKey),
	}
	if clientKey != "" {
		keysToTry = append(keysToTry, fmt.Sprintf("%s|Client:", baseKey))
	}

	for _, key := range keysToTry {
		if vendorData, ok := vendors[key]; ok {
			if status, ok := vendorData["Status"].(int64); ok && status == variables.Active {
				return true
			}
		}
	}
	return false
}

func ShouldHitVendor(client, channel string) bool {
	clientDetails, found := cache.GetCache().GetMappedData(cache.ClientsData)
	if !found {
		utils.Error(fmt.Errorf("client details not found in cache"))
	}
	key := fmt.Sprintf("Name:%s|Channel:%s", client, channel)
	if clientData, ok := clientDetails[key]; ok {
		if status, ok := clientData["ShouldHitVendor"].(int64); ok && status == variables.Active {
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
	if val, ok := data["TemplateEntityId"].(int64); ok {
		req.TemplateEntityId = strconv.FormatInt(val, 10)
	}
	if val, ok := data["TemplateHeader"].(string); ok {
		req.TemplateHeader = val
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
	if val, ok := data["TemplateVariables"].(string); ok {
		req.TemplateVariables = val
	}
	if val, ok := data["TemplateCategory"].(int64); ok {
		req.TemplateCategory = fmt.Sprintf("%d", val)
	}
	if val, ok := data["SmsFallbackVariables"].(string); ok {
		req.SmsFallbackVariables = val
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

// HandleTemplateNotFoundError handles the common template not found error pattern
func HandleTemplateNotFoundError(msg sdkModels.CommApiRequestBody, err error) (bool, map[string]interface{}, error) {
	LogTemplateNotFound(msg, err)
	errorMessage := "template not found for mobile: " + msg.Mobile
	if strings.TrimSpace(msg.TemplateReference) != "" {
		errorMessage += " for template reference: " + strings.TrimSpace(msg.TemplateReference)
	} else {
		errorMessage += " for stage: " + fmt.Sprintf("%.2f", msg.Stage)
	}
	if updateErr := UpdateRedisErrorMessage(msg, errorMessage); updateErr != nil {
		utils.Error(fmt.Errorf("failed to update Redis for template not found: %v", updateErr))
	}

	responseMessage := fmt.Sprintf("No template found for the given Process: %s, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Client, msg.Channel, msg.Vendor)
	if strings.TrimSpace(msg.TemplateReference) != "" {
		responseMessage = fmt.Sprintf("No template found for reference: %s, Process: %s, Client: %s, Channel: %s and Vendor: %s",
			strings.TrimSpace(msg.TemplateReference), msg.ProcessName, msg.Client, msg.Channel, msg.Vendor)
	} else {
		responseMessage = fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s",
			msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor)
	}

	dbResponse := map[string]interface{}{
		"CommId":          msg.CommId,
		"Vendor":          msg.Vendor,
		"MobileNumber":    msg.Mobile,
		"IsSent":          false,
		"ResponseMessage": responseMessage,
	}
	return true, dbResponse, nil // message processed but not sent as Template not found
}

// HandleShouldHitVendorOffError handles the common shouldHitVendor is off error pattern
func HandleShouldHitVendorOffError(msg sdkModels.CommApiRequestBody) error {
	errorMessage := fmt.Sprintf("shouldHitVendor is off for mobile: %s and channel: %s", msg.Mobile, msg.Channel)
	return UpdateRedisErrorMessage(msg, errorMessage)
}
