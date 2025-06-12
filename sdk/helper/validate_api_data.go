package sdkHelper

import (
	"strings"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// ValidateCommRequest validates the CommApi request fields
func ValidateCommRequest(data sdkModels.CommApiRequestBody) (bool, string) {
	// Trim inputs to avoid issues with spaces
	data.Channel = strings.ToUpper(strings.TrimSpace(data.Channel))
	data.Mobile = strings.TrimSpace(data.Mobile)
	data.Email = strings.TrimSpace(data.Email)
	data.ProcessName = strings.ToUpper(strings.TrimSpace(data.ProcessName))

	// Check if channel is provided
	if data.Channel == "" {
		return false, "Channel is required"
	}

	if data.ProcessName == "" {
		return false, "ProcessName is required"
	}

	// Validate based on channel

	switch data.Channel {
	case variables.SMS:
		if data.Mobile == "" {
			return false, "Mobile is required for SMS communication"
		}
	case variables.RCS:
		if data.Mobile == "" {
			return false, "Mobile is required for RCS communication"
		}
	case variables.WhatsApp:
		if data.Mobile == "" {
			return false, "Mobile is required for WhatsApp Communication"
		}

	case variables.Email:
		if data.Email == "" {
			return false, "Email is required for Email communication"
		}
	default:
		return false, "Invalid Channel"
	}

	return true, "Success" // Validation successful
}
