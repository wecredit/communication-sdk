package sinchEmailPayload

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func GetTemplatePayload(data extapimodels.EmailRequestBody, config models.Config) (map[string]interface{}, error) {
	// Map template variables to data field accessors
	varMapping := map[string]func() interface{}{
		"first_name": func() interface{} { return data.CustomerName },
		"due_date":   func() interface{} { return data.DueDate },
		"loan_id":    func() interface{} { return data.LoanId },
		// "amount":     func() interface{} { return data.EmiAmount }, // Added for potential 4th variable
	}

	// Count variables for exact map capacity
	varCount := 0
	if len(data.TemplateVariables) > 0 {
		varCount = len(strings.Split(data.TemplateVariables, ",")) // Approximate count
	}

	// Preallocate attributes map with exact or capped capacity
	attributes := make(map[string]interface{}, min(varCount, len(varMapping)))

	// Process comma-separated TemplateVariables
	if varCount > 0 {
		for _, varName := range strings.Split(data.TemplateVariables, ",") {
			varName = strings.TrimSpace(varName) // Handle potential whitespace
			if getter, exists := varMapping[varName]; exists {
				attributes[varName] = getter()
			} else {
				return nil, fmt.Errorf("unrecognized template variable: %s", varName)
			}
		}
	}

	// Construct payload with minimal allocations
	templatePayload := map[string]interface{}{
		"subject": data.EmailSubject, // subject of email
		"from": map[string]interface{}{
			"email": data.FromEmail, // from email
			"name":  "CreditSea",    // name to be shown in email
		},
		"reply_to": map[string]interface{}{
			"email": "help@creditsea.com", // reply to email
			"name":  "CreditSea",          // reply to name
		},
		"recipients": []map[string]interface{}{
			{
				"to": []map[string]interface{}{
					{
						"email": data.ToEmail,
						"name":  data.CustomerName,
					},
				},
				"attributes": attributes,
				"unique_arguments": map[string]interface{}{
					"x-apiheader": strconv.Itoa(helper.GenerateRandomID(100000, 999999)),
				},
			},
		},
		"template_id": data.TemplateId,
	}

	return templatePayload, nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
