package main

import (
	"fmt"
	"log"
	"os"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func main() {

	if err := config.LoadConfigs(); err != nil {
		utils.Error(fmt.Errorf("failed to load configs: %v", err))
	}

	// For Zapcash UAT testing
	username := os.Getenv("ZAPCASH_USERNAME")
	password := os.Getenv("ZAPCASH_PASSWORD")
	channel := "RCS"
	baseUrl := "http://localhost:8080"

	client, err := sdk.NewSdkClient(username, password, channel, baseUrl)
	// client, err := sdk.NewSdkClient("wecredit", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	// All stage values
	stages := []float64{
		11.01,
		// 1.01, 1.02, 1.03, 1.04, 1.05,
		// 2.01, 2.02, 2.03, 2.04, 2.05, 2.06, 2.07, 2.08,
		// 3.01, 3.02,
		// 4.01, 4.02, 4.03, 4.04, 4.05,
		// 5.01, 5.02,
		// 6.01, 6.02, 6.03, 6.04,
		// 7.01, 7.02, 7.03, 7.04,
		// 8.01, 8.02, 8.03, 8.04,
		// 9.01, 9.02, 9.03, 9.04, 9.05,
		// 10.01, 10.02, 10.03, 10.04, 10.05,
		// 11.01, 11.02, 11.03, 11.04, 11.05, 11.06, 11.07,
		// 12.01, 12.02, 12.03, 12.04, 12.05, 12.06, 12.07, 12.08, 12.09, 12.10,
		// 12.11, 12.12, 12.13, 12.14, 12.15, 12.16, 12.17, 12.18, 12.19, 12.20,
		// 12.21, 12.22, 12.23, 12.24, 12.25, 12.26, 12.27, 12.28, 12.29, 12.30,
		// 12.31, 12.32,
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
			DbClient:           database.DBtechWrite,
			InputTableName:     "RcsInputAuditTable",
			Mobile:             "9220146969",
			Email:              "nikhil@wecredit.co.in",
			Channel:            "RCS",
			ProcessName:        "ZAPCASH",
			Stage:              stage,
			IsPriority:         true,
			EmiAmount:          "25000",
			CustomerName:       "Vaibhav",
			LoanId:             "1234616232324",
			ApplicationNumber:  "2696944656976",
			DueDate:            "2026-04-20",
			Description:        fmt.Sprintf("TEST for stage %.2f", stage),
			TotalPayableAmount: "100000",
			TodayPayableAmount: "90000",
			SavingAmount:       "10000",
			BounceCharge:       "5000",
			PaymentLink:        "https://www.google.com",
		}

		response, err := client.Send(request)
		if err != nil {
			log.Printf("❌ Failed to send for stage %.2f: %v", stage, err)
			continue
		}

		log.Printf("✅ Sent successfully for stage %.2f: %+v\n", stage, response)
	}
}
