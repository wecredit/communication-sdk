package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

var Configs models.Config

func LoadConfigs() error {
	// Load the .env file (optional, for local development)
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			return fmt.Errorf("error loading .env file: %v", err)
		}
	}

	// Use reflection to set the struct fields with environment variables
	val := reflect.ValueOf(&Configs).Elem() // Pass a pointer to the struct
	typ := reflect.TypeOf(Configs)          // Use the struct type (not the pointer)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		envVar := typ.Field(i).Tag.Get("envconfig")

		if value, exists := os.LookupEnv(envVar); exists {
			if field.CanSet() {
				field.SetString(value)
			}
		} else {
			// Set default value if available
			defaultVal := typ.Field(i).Tag.Get("default")
			if defaultVal != "" {
				if field.CanSet() {
					field.SetString(defaultVal)
				}
			}
		}
	}

	if err := ValidateTimesWpMode(Configs); err != nil {
		return err
	}

	if err := ratelimit.InitFromConfig(Configs.ProviderRPSOverrides, Configs.ProviderRPSApprovedCaps, Configs.ProviderRPSDefault); err != nil {
		return fmt.Errorf("invalid PROVIDER_RPS_OVERRIDES vs PROVIDER_RPS_APPROVED_CAPS: %w", err)
	}

	logChannelAuditToggles()

	// Initialize Redis Connection
	_, err := redis.GetRedisClient(Configs.RedisAddress, Configs.RedisPassword)
	if err != nil {
		utils.Error(fmt.Errorf("failed to initialize redis connection"))
	}

	// Initialize the creditsea whatsapp current count of the day on redis, to check if the creditsea whatsapp count is exceeded
	currentCountInt, _ := strconv.Atoi(Configs.CreditSeaWhatsappCurrentCount)
	redis.InitCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount, currentCountInt)

	/* Commented out because we are not using Analytics DB for now
	// Connect Analytics DB
	err := database.ConnectDB(database.Analytics, Configs)
	if err != nil {
		return fmt.Errorf("failed to initialize Analytics database: %v", err)
	}
	utils.Info("Analytics Database connection pool initialized successfully.")
	*/

	// Connect Tech DB
	err = database.ConnectDB(database.Tech, Configs)
	if err != nil {
		return fmt.Errorf("failed to initialize Tech database: %v", err)
	}
	utils.Info("Tech Database connection pool initialized successfully.")

	if err := database.ConnectDB(database.Marketing, Configs); err != nil {
		return fmt.Errorf("failed to initialize Marketing database: %v", err)
	}

	if err := database.ConnectZapCashV1IfEnabled(Configs); err != nil {
		utils.Error(fmt.Errorf("zapcash v1 audit database unavailable: %v", err))
	}

	// Configure Queue client
	if err := queue.InitAWSClients(Configs.AWSRegion); err != nil {
		return fmt.Errorf("failed to initialize AWS clients: %v", err)
	}

	/*
		_, err = queue.GetSdkSnsClient(Configs.AWSRegion)
		if err != nil {
			utils.Error(fmt.Errorf("failed to initialize SDK Client: %v", err))
			return err
		} else {
			utils.Info("SNS Client initialized successfully.")
		}
	*/

	return nil
}

// logChannelAuditToggles records the resolved WeCredit, ZapCash, and Core write flags.
func logChannelAuditToggles() {
	database.SetChannelAuditFlags(Configs.CommInputOutputWriteWeCredit, Configs.CommInputOutputWriteZapCash, Configs.ZapCashV1InputOutputWrite)

	utils.Info(fmt.Sprintf("channel audit toggles: comm_wecredit=%t comm_zapcash=%t zapcash_v1=%t",
		database.CommWriteEnabled(variables.WeCredit, Configs.CommInputOutputWriteWeCredit, Configs.CommInputOutputWriteZapCash),
		database.CommWriteEnabled(variables.ZapCash, Configs.CommInputOutputWriteWeCredit, Configs.CommInputOutputWriteZapCash),
		database.ZapCashV1WriteEnabled(Configs.ZapCashV1InputOutputWrite),
	))
}

const (
	TimesWpModeSync  = "sync"
	TimesWpModeAsync = "async"
)

// ResolveTimesWpMode returns sync or async. A blank value is sync.
func ResolveTimesWpMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return TimesWpModeSync, nil
	}
	if mode != TimesWpModeSync && mode != TimesWpModeAsync {
		return "", fmt.Errorf("TIMES_WP_API_MODE must be sync or async")
	}
	return mode, nil
}

// TimesWpSendURL returns the sync or async send URL for mode.
// Async with a blank URL is an error. Sync keeps the configured sync URL, including blank.
func TimesWpSendURL(mode, syncURL, asyncURL string) (string, error) {
	if mode == TimesWpModeAsync {
		asyncURL = strings.TrimSpace(asyncURL)
		if asyncURL == "" {
			return "", fmt.Errorf("TIMES_WP_ASYNC_API_URL is required when TIMES_WP_API_MODE=async")
		}
		return asyncURL, nil
	}
	return strings.TrimSpace(syncURL), nil
}

// TimesWpRateChannel is the limiter channel segment for mode.
// Sync stays whatsapp. Async uses whatsapp-async so the 200 cap does not raise the sync cap.
func TimesWpRateChannel(mode string) string {
	if mode == TimesWpModeAsync {
		return "whatsapp-async"
	}
	return "whatsapp"
}

// TimesTemplateListBaseURL is the origin for the Times template get-list call.
// An explicit list base wins. Otherwise the host of the sync send URL is used.
func TimesTemplateListBaseURL(listBase, syncURL string) string {
	if v := strings.TrimSpace(listBase); v != "" {
		return strings.TrimRight(v, "/")
	}
	raw := strings.TrimSpace(syncURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// ValidateTimesWpMode fails process startup when async mode has no URL, the mode
// is unknown, or template-sync single-host fallback has no list base.
func ValidateTimesWpMode(cfg models.Config) error {
	mode, err := ResolveTimesWpMode(cfg.TimesWpApiMode)
	if err != nil {
		return err
	}
	if _, err = TimesWpSendURL(mode, cfg.TimesWpApiUrl, cfg.TimesWpAsyncApiUrl); err != nil {
		return err
	}

	syncOn := strings.EqualFold(strings.TrimSpace(cfg.WhatsappTemplateSyncEnabled), "true")
	fallbackOn := strings.EqualFold(strings.TrimSpace(cfg.WhatsappTemplateSyncAllowSingleHostFallback), "true")
	if syncOn && fallbackOn && TimesTemplateListBaseURL(cfg.TimesWpTemplateListBaseUrl, cfg.TimesWpApiUrl) == "" {
		return fmt.Errorf("TIMES_WP_TEMPLATE_LIST_BASE_URL or TIMES_WP_API_URL is required when template sync single-host fallback is enabled")
	}
	return nil
}
