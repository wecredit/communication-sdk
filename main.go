package main

import (
	"fmt"
	"log"
	"os"

	"github.com/wecredit/communication-sdk/config"
	pinnacleSms "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func main() {
	if err := config.LoadConfigs(); err != nil {
		utils.Error(fmt.Errorf("failed to load configs: %v", err))
		log.Fatalf("config load failed: %v", err)
	}

	runPinnacleTemplateTests()
	runLegacySdkSmokeTest()
}

func runLegacySdkSmokeTest() {
	username := os.Getenv("WECREDIT_USERNAME")
	password := os.Getenv("WECREDIT_PASSWORD")
	fmt.Printf("Username: %s, Password: %s\n", username, password) // For debugging
	channel := "SMS"
	baseUrl := "http://localhost:8080"

	client, err := sdk.NewSdkClient(username, password, channel, baseUrl)
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
		return
	}

	fmt.Println("\nClient Created:", client)

	stages := []float64{11.01}

	for _, stage := range stages {
		request := &sdkModels.CommApiRequestBody{
			DbClient:           database.DBtechWrite,
			InputTableName:     "SmsInputAuditTable",
			Mobile:             "7014850582",
			Email:              "nikhil@wecredit.co.in",
			Channel:            "SMS",
			ProcessName:        "Wecredit",
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

func runPinnacleTemplateTests() {
	tests := []struct {
		name          string
		sender        string
		message       string
		dltTemplateID int64
		dltEntityID   string
		mobile        string
		processName   string
		client        string
	}{
		{
			name:          "WECRFC",
			sender:        "WECRFC",
			message:       "Dekhiye TrueBalance se aap kitne loan ke liye eligible hain. Rs 2 lakh tak ka loan option available hai. Explore karein {#urg#} WeCredit",
			dltTemplateID: 1777178523379905176,
			dltEntityID:   "1705171498519732707",
			mobile:        "7014850582",
			processName:   "WECRFC_TEST",
			client:        "wecredit",
		},
		{
			name:          "WECRLA",
			sender:        "WECRLA",
			message:       "Loan ab fast aur simple LNT se voh Rs 15 lakh tak. 0.9 percent interest se start. Aaj hi apply karein ghar baithe {#urg#} WeCredit",
			dltTemplateID: 1777178512993384044,
			dltEntityID:   "1705177148335427485",
			mobile:        "7014850582",
			processName:   "WECRLA_TEST",
			client:        "wecredit",
		},
	}

	for _, tc := range tests {
		config.Configs.TimesSmsApiSender = tc.sender
		config.Configs.PinnacleSmsDltEntityId = tc.dltEntityID

		req := extapimodels.SmsRequestBody{
			Client:            tc.client,
			Process:           tc.processName,
			Mobile:            tc.mobile,
			DltTemplateId:     tc.dltTemplateID,
			TemplateText:      tc.message,
			Description:       tc.message,
			TemplateVariables: "",
		}

		urlStr, err := pinnacleSms.BuildPinnacleURL(config.Configs.PinnacleSmsApiUrl, config.Configs.PinnacleSmsAccessKey, tc.sender, tc.mobile, tc.message, tc.dltTemplateID, tc.dltEntityID)
		if err != nil {
			log.Printf("❌ Could not build URL for %s: %v", tc.name, err)
			continue
		}

		fmt.Printf("\n=== %s ===\n", tc.name)
		fmt.Printf("URL: %s\n", urlStr)
		fmt.Printf("curl: curl -sS -L '%s'\n", urlStr)

		resp := pinnacleSms.HitPinnacleApi(req)
		fmt.Printf("sent=%v transactionId=%q response=%s\n", resp.IsSent, resp.TransactionId, resp.ResponseMessage)
	}
}
