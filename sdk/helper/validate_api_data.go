package helper

// import (
// 	"strings"

// 	models "github.com/wecredit/communication-sdk/models/api"
// )

// const (
// 	Sms      = "SMS"
// 	Email    = "Email"
// 	WhatsApp = "WhatsApp"
// )

// // ValidateCommRequest validates the CommApi request fields
// func ValidateCommRequest(data models.CommApiRequestBody) (bool, string) {
// 	// Trim inputs to avoid issues with spaces
// 	commType := strings.TrimSpace(data.Channel)
// 	mobile := strings.TrimSpace(data.Mobile)
// 	email := strings.TrimSpace(data.Email)
// 	processName := strings.TrimSpace(data.ProcessName)

// 	// Check if CommType is provided
// 	if commType == "" {
// 		return false, "CommType is required"
// 	}

// 	if processName == "" {
// 		return false, "ProcessName is required"
// 	}

// 	// Validate based on CommType
// 	switch commType {
// 	case Sms:
// 		if mobile == "" {
// 			return false, "Mobile is required for SMS communication"
// 		}
// 	case Email:
// 		if email == "" {
// 			return false, "Email is required for Email communication"
// 		}
// 	case WhatsApp:
// 		if mobile == "" {
// 			return false, "Mobile and ProcessName are required for WhatsApp communication"
// 		}
// 	default:
// 		return false, "Invalid CommType"
// 	}

// 	return true, "Success" // Validation successful
// }
