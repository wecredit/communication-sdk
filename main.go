package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("required environment variable %s is not set", name)
	}
	return value
}

func optionalEnv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func main() {
	if err := config.LoadConfigs(); err != nil {
		utils.Error(fmt.Errorf("failed to load configs: %v", err))
	}

	// Local ZapCash harness. Do not run the nurture feed poller against prod for UAT.
	// ZAPCASH_TEST_CHANNEL selects PUSH, RCS, WHATSAPP, SMS, EMAIL, or IVR.
	username := os.Getenv("ZAPCASH_USERNAME")
	password := os.Getenv("ZAPCASH_PASSWORD")
	channel := strings.ToUpper(optionalEnv("ZAPCASH_TEST_CHANNEL"))
	if channel == "" {
		channel = "RCS"
	}
	switch channel {
	case "PUSH", "RCS", "WHATSAPP", "SMS", "EMAIL", "IVR":
	default:
		log.Fatalf("ZAPCASH_TEST_CHANNEL must be PUSH, RCS, WHATSAPP, SMS, EMAIL, or IVR")
	}

	baseURL := "http://localhost:8080"
	customerName := requiredEnv("ZAPCASH_TEST_CUSTOMER_NAME")
	applicationNumber := requiredEnv("ZAPCASH_TEST_APPLICATION_NUMBER")
	loanID := requiredEnv("ZAPCASH_TEST_LOAN_ID")

	mobile := ""
	email := ""
	var deviceTokens []string
	switch channel {
	case "EMAIL":
		email = requiredEnv("ZAPCASH_TEST_EMAIL")
	case "PUSH":
		deviceTokens = []string{requiredEnv("ZAPCASH_FCM_DEVICE_TOKEN")}
	default:
		// IVR uses the same mobile as RCS, SMS, and WhatsApp. The consumer
		// posts that call with CustCall_ID set to CommId.
		mobile = requiredEnv("ZAPCASH_TEST_MOBILE")
	}

	client, err := sdk.NewSdkClient(username, password, channel, baseURL)
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v\n", err)
		return
	}
	fmt.Println("\nClient Created:", client)

	// Uncomment the stages that have a ZapCash template for this channel.
	// Stages 8 and 10 and stages 13–15 are intentionally not configured for PUSH.
	stages := []float64{
		// 1.01,
		// 1.02,
		// 1.03,
		// 2.01,
		// 2.02,
		// 2.03,
		// 3.01,
		// 3.02,
		// 3.03,
		// 4.01,
		// 4.02,
		// 4.03,
		// 5.01,
		// 5.02,
		// 5.03,
		// 6.01,
		// 6.02,
		// 6.03,
		// 7.01,
		// 7.02,
		// 7.03,
		// 9.01,
		// 9.02,
		// 9.03,
		// 11.01,
		// 11.02,
		// 11.03,
		// 12.01,
		// 12.02,
		// 12.03,
		// 12.04,
		// 12.05,
		// 12.06,
		// 12.07, 12.08, 12.09,
		// 12.10, 12.11, 12.12, 12.13, 12.14, 12.15, 12.16,
		// 12.17, 12.18, 12.19, 12.20, 12.21, 12.22, 12.23, 12.24,
		// 12.25, 12.26, 12.27, 12.28, 12.29, 12.30,
		// 12.31,
		// 13.01,
	}

	// Empty vendor lets the consumer assign the active vendor for this client and channel.
	// Set ZAPCASH_TEST_VENDOR only when the test must pin one vendor.
	vendor := optionalEnv("ZAPCASH_TEST_VENDOR")
	runID := time.Now().UTC().Format("20060102T150405.000000000")
	for _, stage := range stages {
		eventID := fmt.Sprintf("test-%s-v1-%s-%.2f", strings.ToLower(channel), runID, stage)
		request := &sdkModels.CommApiRequestBody{
			DbClient: database.DBtechWrite,
			// InputTableName stays empty. sdk.Send writes SMS, RCS, and email input rows
			// when it is set, and the channel handler writes that same table again.
			Mobile:             mobile,
			Email:              email,
			Channel:            channel,
			Client:             "zapcash",
			ProcessName:        "ZAPCASH",
			Vendor:             vendor,
			Stage:              stage,
			IsPriority:         true,
			EventId:            eventID,
			CampaignDate:       "2026-09-07",
			CustomerName:       customerName,
			ApplicationNumber:  applicationNumber,
			LoanId:             loanID,
			DueDate:            "2026-04-02",
			Description:        fmt.Sprintf("%s TEST for stage %.2f", channel, stage),
			DeviceTokens:       deviceTokens,
			EmiAmount:          "25000",
			TotalPayableAmount: "100000",
			TodayPayableAmount: "90000",
			SavingAmount:       "10000",
			BounceCharge:       "5000",
			PaymentLink:        "https://www.google.com",
		}

		response, err := client.Send(request)
		if err != nil {
			log.Printf("Failed to send %s for stage %.2f: %v", channel, stage, err)
			continue
		}
		log.Printf("Sent %s successfully for stage %.2f eventId=%s: %+v\n", channel, stage, eventID, response)
	}
}
