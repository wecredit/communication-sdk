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

	// ZapCash PUSH UAT placeholders — replace before running.
	username := os.Getenv("ZAPCASH_USERNAME")
	password := os.Getenv("ZAPCASH_PASSWORD")
channel := "PUSH"	baseURL := "http://localhost:8080"
	deviceToken := "f3ZSUamCQPS64eCXlu2-VL:APA91bErwa3sXWts0QW94yFL-VYioHlT5ZEblyoGZ0X24AfLyPl8KfmN7SkLUtISnFu6CTEbbE4kggv2YfNzkUCdr7F9DTVL2Qb_UJk3wSWhPLhh1qIsZSM"

	client, err := sdk.NewSdkClient(username, password, channel, baseURL)
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v\n", err)
		return
	}
	fmt.Println("\nClient Created:", client)

	// ZapCash PUSH stages (DAY → .01 / .02 / .03). Uncomment groups as needed.
	stages := []float64{
		// OTP done but BD not done
		1.01, 1.02, 1.03,
		// experian to banking
		// 2.01, 2.02, 2.03,
		// Offer View
		// 3.01, 3.02, 3.03,
		// Offer Accepted to E sign
		// 4.01, 4.02, 4.03,
		// payment credited (Instant)
		// 5.01,
		// Document requested / rejected (Instant)
		// 6.01, 7.01,
		// reloan
		// 9.01, 9.02, 9.03,
		// foreclose
		// 10.01, 10.02, 10.03,
		// DueDate (D0)
		// 11.01,
		// Overdue dpd 1 to 5
		// 12.01, 12.02, 12.03,
		// Overdue 6 to 15
		// 13.01, 13.02, 13.03,
		// Overdue 15 to 30 / 30+
		// 14.01, 15.01,
	}

	for _, stage := range stages {
		eventID := fmt.Sprintf("test-push-v1%.2f", stage)
		request := &sdkModels.CommApiRequestBody{
			DbClient:           database.DBtechWrite,
			InputTableName:     "", // PUSH audit is written by consumer handlePush, not SDK Send
			Mobile:             "8888888888",
			Channel:            "PUSH",
			Client:             "zapcash",
			ProcessName:        "zapcash",
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
