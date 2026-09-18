package main

import (
	"fmt"
	"log"
	"os"
	"time"

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

	// ZapCash PUSH harness — do not run the nurture feed poller against prod for UAT.
	// Replace deviceToken with a live FCM registration for project zapcash-901a1.
	username := os.Getenv("ZAPCASH_USERNAME")
	password := os.Getenv("ZAPCASH_PASSWORD")
	channel := "PUSH"
	baseURL := "http://localhost:8080"
	deviceToken := "REPLACE_WITH_LIVE_FCM_DEVICE_TOKEN"

	client, err := sdk.NewSdkClient(username, password, channel, baseURL)
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v\n", err)
		return
	}
	fmt.Println("\nClient Created:", client)

	// Stages match the full ZapCash PUSH UAT seed in TemplateDetails.
	// Stages 8 and 10 and stages 13–15 are intentionally not configured.
	stages := []float64{
		1.01, 1.02, 1.03,
		2.01, 2.02, 2.03,
		3.01, 3.02, 3.03,
		4.01, 4.02, 4.03,
		5.01, 5.02, 5.03,
		6.01, 6.02, 6.03,
		7.01, 7.02, 7.03,
		9.01, 9.02, 9.03,
		11.01, 11.02, 11.03,
		12.01, 12.02, 12.03, 12.04, 12.05, 12.06, 12.07, 12.08,
		12.09, 12.10, 12.11, 12.12, 12.13, 12.14, 12.15, 12.16,
		12.17, 12.18, 12.19, 12.20, 12.21, 12.22, 12.23, 12.24,
		12.25, 12.26, 12.27, 12.28, 12.29, 12.30, 12.31,
	}

	runID := time.Now().UTC().Format("20060102T150405.000000000")
	for _, stage := range stages {
		eventID := fmt.Sprintf("test-push-v1-%s-%.2f", runID, stage)
		request := &sdkModels.CommApiRequestBody{
			DbClient:           database.DBtechWrite,
			InputTableName:     "", // PUSH audit is written by consumer handlePush, not SDK Send
			Mobile:             "8888888888",
			Channel:            "PUSH",
			Client:             "zapcash",
			ProcessName:        "ZAPCASH",
			Vendor:             "FCM",
			Stage:              stage,
			IsPriority:         true,
			EventId:            eventID,
			CampaignDate:       "2026-09-07",
			CustomerName:       "Ronit",
			ApplicationNumber:  "2696944656976",
			LoanId:             "1234616232324",
			DueDate:            "2026-04-20",
			Description:        fmt.Sprintf("PUSH TEST for stage %.2f", stage),
			DeviceTokens:       []string{deviceToken},
			EmiAmount:          "25000",
			TotalPayableAmount: "100000",
			TodayPayableAmount: "90000",
			SavingAmount:       "10000",
			BounceCharge:       "5000",
			PaymentLink:        "https://www.google.com",
		}

		response, err := client.Send(request)
		if err != nil {
			log.Printf("Failed to send PUSH for stage %.2f: %v", stage, err)
			continue
		}
		log.Printf("Sent PUSH successfully for stage %.2f eventId=%s: %+v\n", stage, eventID, response)
	}

	/*
		// Previous RCS harness (kept for reference):
		username := "REPLACE_ZAPCASH_USERNAME"
		password := "REPLACE_ZAPCASH_PASSWORD"
		client, err := sdk.NewSdkClient(username, password, "RCS", "http://localhost:8080")
		stages := []float64{1.01}
		for _, stage := range stages {
			request := &sdkModels.CommApiRequestBody{
				DbClient:           database.DBtechWrite,
				InputTableName:     "RcsInputAuditTable",
				Mobile:             "7014850582",
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
				log.Printf("Failed RCS stage %.2f: %v", stage, err)
				continue
			}
			log.Printf("Sent RCS stage %.2f: %+v\n", stage, response)
		}
	*/
}
