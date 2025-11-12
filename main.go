package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {
	// For Creditsea local testing
	username := "wecredit"
	password := "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU="
	channel := "SMS"
	baseUrl := "http://localhost:8080"

	// For Creditsea UAT testing
	// username := "creditsea"
	// password := "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9"
	// channel := "WHATSAPP"
	// baseUrl := "http://172.16.23.114:8080"

	// For Nurture Engine local testing
	// username := "wecredit"
	// password := "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU="
	// channel := "SMS"
	// baseUrl := "http://172.16.23.114:8080"

	client, err := sdk.NewSdkClient(username, password, channel, baseUrl)
	// client, err := sdk.NewSdkClient("wecredit", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	// All stage values
	stages := []float64{
		1.02, //1.02, 1.03, 1.04,
		// 2.01, 2.02, 2.03, 2.04, 2.05, 2.06,
		// 3.01, 3.02, 3.03, 3.04,
		// 4.01, 4.02, 4.03, 4.04,
		// 5.01, 5.02, 5.03, 5.04, 5.05, 5.06,
		// 6.01, 6.02, 6.03, 6.04, 6.05, 6.06, 6.07, 6.08, 6.09, 6.10, 6.11, 6.12, 6.13, 6.14, 6.15, 6.16, 6.17, 6.18, 6.19, 6.20,
		// 7.01, 7.02, 7.03, 7.04, 7.05, 7.06,
		// 8.01, 8.02,
	}

	// // All stage values
	// stages := []float64{
	// 	// 1.05,1.06,1.07,1.08,1.09,1.10,
	// 	// 2.07,2.08,2.09,
	// 	// 3.05,3.06,
	// 	// 8.01,
	// }

	// Loop through each stage and send email
	for _, stage := range stages {
		request := &sdkModels.CommApiRequestBody{
			Mobile:            "7570897034",
			Email:             "nikhil@wecredit.co.in",
			Channel:           "SMS",
			ProcessName:       "OLYV",
			Stage:             stage,
			IsPriority:        true,
			EmiAmount:         "25000",
			CustomerName:      "Nikhil",
			LoanId:            "1234616232324",
			ApplicationNumber: "2696944656976",
			DueDate:           "2025-10-20",
			Description:       fmt.Sprintf("TEST for stage %.2f", stage),
		}

		response, err := client.Send(request)
		if err != nil {
			log.Printf("❌ Failed to send for stage %.2f: %v", stage, err)
			continue
		}

		log.Printf("✅ Sent successfully for stage %.2f: %+v\n", stage, response)
	}
}
