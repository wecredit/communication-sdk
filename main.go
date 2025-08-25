package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {
	// For Creditsea local testing
	// username := "creditsea"
	// password := "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9"
	// channel := "WHATSAPP"
	// baseUrl := "http://localhost:8080"

	// For Creditsea UAT testing
	// username := "creditsea"
	// password := "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9"
	// channel := "EMAIL"
	// baseUrl := "http://172.16.18.217:8080"

	// For Nurture Engine local testing
	username := "wecredit"
	password := "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU="
	channel := "SMS"
	baseUrl := "http://172.16.24.13:8080"

	client, err := sdk.NewSdkClient(username, password, channel, baseUrl)
	// client, err := sdk.NewSdkClient("wecredit", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	// All stage values
	stages := []float64{
		2.01, 2.02, 2.03, 2.04, 2.05, 2.06, 2.07, 2.08,
	// 	4.01, 4.02, 4.03, 4.04, 4.05, 4.06,
	// 	5.01, 5.02, 5.03, 5.04, 5.05, 5.06, 5.07,
	// 	6.01, 6.02, 6.03, 6.04, 6.05, 6.06, 6.07, 6.08, 6.09, 6.10,
	// 	6.11, 6.12, 6.13, 6.14, 6.15, 6.16, 6.17, 6.18, 6.19, 6.20,
	// 	6.21, 6.22, 6.23, 6.24, 6.25, 6.26, 6.27, 6.28, 6.29, 6.30,
	// 	6.31, 6.32, 6.33, 6.34, 6.35, 6.36, 6.37, 6.38, 6.40, 6.41,
	// 	6.42, 6.43, 6.44, 6.45, 6.46, 6.47, 6.48, 6.49, 6.50, 6.51,
	// 	6.52, 6.53, 6.54, 6.55, 6.56,
	// 	7.01, 7.02, 7.03, 7.04, 7.05, 7.06, 7.07, 7.08, 7.09,
	// 	8.01, 8.02, 8.03, 8.04, 8.05, 8.06, 8.07, 8.08, 8.09,
	}

	// All stage values
	// stages := []float64{
	// 	2.01, //1.02, // 1.03, 1.04, 1.05, 1.06, 1.07, 1.08, 1.09, 1.10,
	// 	// 2.01, 2.02, 2.03, 2.04, 2.05, 2.06, 2.07, 2.08, 2.09, 2.10,
	// 	// 3.01, 3.02, 3.03, 3.04, 3.05, 3.06,
	// 	// 7.01, 7.02, 7.03, 7.04, 7.05,
	// }

	// Loop through each stage and send email
	for _, stage := range stages {
		request := &sdkModels.CommApiRequestBody{
			Mobile:            "6200807541",
			Email:             "honeydoultani@creditsea.com",
			Channel:           "SMS",
			ProcessName:       "OLYV",
			Stage:             stage,
			IsPriority:        true,
			EmiAmount:         "564746",
			CustomerName:      "Honey",
			LoanId:            "8727332869",
			ApplicationNumber: "632563232469",
			DueDate:           "2025-08-20",
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
