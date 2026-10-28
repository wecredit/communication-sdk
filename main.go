package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
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

	vendor := optionalEnv("ZAPCASH_TEST_VENDOR")
	stages, err := zapCashTestStages(channel, optionalEnv("ZAPCASH_TEST_STAGES"), vendor)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Testing %d active ZapCash %s template stage(s)", len(stages), channel)
	paymentLink := ""
	if channel == "WHATSAPP" {
		paymentLink = optionalEnv("ZAPCASH_TEST_PAYMENT_LINK")
		if paymentLink == "" {
			log.Print("ZAPCASH_TEST_PAYMENT_LINK is empty; payment-link stages will use each template's configured button URL")
		}
	}

	// Empty vendor lets the consumer assign the active vendor for this client and channel.
	// Set ZAPCASH_TEST_VENDOR only when the test must pin one vendor.
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
			PaymentLink:        paymentLink,
		}

		response, err := client.Send(request)
		if err != nil {
			log.Printf("Failed to send %s for stage %.2f: %v", channel, stage, err)
			continue
		}
		log.Printf("Queued %s test for stage %.2f eventId=%s: %+v\n", channel, stage, eventID, response)
	}
}

func zapCashTestStages(channel, override, vendor string) ([]float64, error) {
	if strings.TrimSpace(override) != "" {
		return parseZapCashTestStages(override)
	}

	switch channel {
	case "PUSH", "IVR", "RCS", "WHATSAPP", "SMS", "EMAIL":
	default:
		return nil, fmt.Errorf("unsupported test channel %q", channel)
	}

	table := strings.TrimSpace(config.Configs.TemplateDetailsTable)
	if table == "" {
		return nil, fmt.Errorf("TEMPLATE_TABLE must be configured to discover active %s test stages", channel)
	}
	if database.DBtechRead == nil {
		return nil, fmt.Errorf("Tech read database is unavailable; set ZAPCASH_TEST_STAGES to provide explicit stages")
	}

	type templateStage struct {
		Stage *float64 `gorm:"column:Stage"`
	}
	var rows []templateStage
	query := database.DBtechRead.Table(table).
		Distinct("Stage").
		Where("Client = ? AND Process = ? AND Channel = ? AND IsActive = ?", "zapcash", "ZAPCASH", channel, true)
	if strings.TrimSpace(vendor) != "" {
		query = query.Where("Vendor = ?", vendor)
	}
	if err := query.Order("Stage ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("discover active ZapCash %s template stages: %w", channel, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no active ZapCash templates found for %s (vendor %q); set ZAPCASH_TEST_STAGES to specify stages", channel, vendor)
	}
	stages := make([]float64, 0, len(rows))
	for _, row := range rows {
		if row.Stage != nil {
			stages = append(stages, *row.Stage)
		}
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("no active ZapCash templates with a non-NULL stage found for %s (vendor %q); set ZAPCASH_TEST_STAGES to specify stages", channel, vendor)
	}
	return stages, nil
}

func parseZapCashTestStages(csv string) ([]float64, error) {
	parts := strings.Split(csv, ",")
	stages := make([]float64, 0, len(parts))
	seen := make(map[float64]struct{}, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 13.99 || math.Abs(value*100-math.Round(value*100)) > 1e-8 || int(math.Round(value*100))%100 == 0 {
			return nil, fmt.Errorf("invalid ZAPCASH_TEST_STAGES value %q; use comma-separated stage values such as 1.01,11.01,13.01", strings.TrimSpace(part))
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		stages = append(stages, value)
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("ZAPCASH_TEST_STAGES contains no stage values")
	}
	return stages, nil
}
