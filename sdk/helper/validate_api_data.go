package helper

import (
	"strings"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// ValidateCommRequest validates the CommApi request fields
func ValidateCommRequest(data sdkModels.CommApiRequestBody) (bool, string) {
	// Trim inputs to avoid issues with spaces
	channel := strings.TrimSpace(data.Channel)
	mobile := strings.TrimSpace(data.Mobile)
	email := strings.TrimSpace(data.Email)
	processName := strings.TrimSpace(data.ProcessName)

	// Check if channel is provided
	if channel == "" {
		return false, "channel is required"
	}

	if processName == "" {
		return false, "ProcessName is required"
	}

	// Validate based on channel
	switch channel {
	case variables.SMS:
		if mobile == "" {
			return false, "Mobile is required for SMS communication"
		}
	case variables.RCS:
		if mobile == "" {
			return false, "Mobile is required for RCS communication"
		}
	case variables.WhatsApp:
		if mobile == "" {
			return false, "Mobile and ProcessName are required for WhatsApp communication"
		}

	case variables.Email:
		if email == "" {
			return false, "Email is required for Email communication"
		}
	default:
		return false, "Invalid Channel"
	}

	return true, "Success" // Validation successful
}
