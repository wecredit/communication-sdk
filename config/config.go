package config

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"reflect"
	"runtime"
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

	// Apply central tuning before consumers and provider channels start. AppConfig
	// values win; environment/code values remain the startup fallback.
	if err := applyRuntimeTuningFromAppConfig(); err != nil {
		return err
	}
	slots := int(positiveConfigInt(Configs.ProviderRPSTaskSlots, 1))
	ratelimit.ConfigureRedis(redis.RDB, slots)
	if err := ratelimit.InitFromConfig(
		Configs.ProviderRPSOverrides,
		Configs.ProviderRPSApprovedCaps,
		Configs.ProviderRPSDefault,
		Configs.ProviderRPSTaskSlots,
	); err != nil {
		return fmt.Errorf("invalid provider RPS configuration: %w", err)
	}

	utils.Info(fmt.Sprintf("provider RPS budget configured task_slots=%d (client totals across tasks)", slots))
	utils.SetLogLevel(Configs.LogLevel)

	if procs := int(positiveConfigInt(Configs.GoMaxProcs, 1)); procs > 0 {
		runtime.GOMAXPROCS(procs)
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

func positiveConfigInt(raw string, fallback int) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return int64(fallback)
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return int64(fallback)
	}
	return n
}

// applyRuntimeTuningFromAppConfig loads operational tuning from AppConfig.
// Blank or unreachable AppConfig keeps code/environment fallbacks and never blocks boot.
func applyRuntimeTuningFromAppConfig() error {
	fallback := Configs
	candidate := Configs
	appConfigValues := make(map[string]string)
	db := database.DBtechWrite
	if db == nil {
		db = database.DBtechRead
	}

	if db == nil {
		utils.Info("AppConfig overlay skipped: communication database is not initialized")
	} else {
		table := strings.TrimSpace(Configs.AppConfigTableName)
		if table == "" {
			table = "AppConfig"
		}

		type appConfigRow struct {
			ConfigKey   string `gorm:"column:ConfigKey"`
			ConfigValue string `gorm:"column:ConfigValue"`
		}

		var rows []appConfigRow
		keys := []string{
			"PROVIDER_RPS_OVERRIDES",
			"CONSUMER_CLIENT_WORKER_OVERRIDES",
			"CONSUMER_CHANNEL_WORKER_OVERRIDES",
			"LOG_LEVEL",
			"COMM_ASYNC_WRITE_CHANNELS",
			"COMM_WRITE_STREAM_BUDGET_OVERRIDES",
		}

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
		query := fmt.Sprintf("SELECT ConfigKey, ConfigValue FROM %s WHERE ConfigKey IN (%s)", table, placeholders)
		args := make([]interface{}, len(keys))
		for i, key := range keys {
			args[i] = key
		}

		if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
			utils.Error(fmt.Errorf("AppConfig overlay load failed; keeping code/environment defaults: %w", err))
		} else {
			byKey := make(map[string]string, len(rows))
			for _, row := range rows {
				key := strings.TrimSpace(row.ConfigKey)
				if key != "" {
					byKey[key] = strings.TrimSpace(row.ConfigValue)
				}
			}

			appConfigValues = byKey
			for _, key := range keys {
				value, exists := byKey[key]
				if !exists {
					utils.Warn("runtime_tuning_missing key=" + key + " source=fallback")
					continue
				}
				if strings.TrimSpace(value) != "" {
					if envValue, envExists := os.LookupEnv(key); envExists && strings.TrimSpace(envValue) != "" {
						utils.Warn("runtime_tuning_duplicate_source key=" + key + " winner=appconfig action=remove_env_from_task_definition")
					}
				}
			}

			ApplyAppConfigValues(&candidate, byKey)
		}
	}

	if err := ratelimit.ValidateOverridesAgainstCaps(candidate.ProviderRPSOverrides, candidate.ProviderRPSApprovedCaps); err != nil {
		utils.Error(fmt.Errorf("runtime provider RPS values invalid; keeping code/environment defaults: %w", err))
		candidate = fallback
		if fallbackErr := validateProviderRPSConfig(fallback); fallbackErr != nil {
			return fmt.Errorf("invalid fallback provider RPS configuration: %w", fallbackErr)
		}
	} else if err := validateProviderRPSDefault(candidate.ProviderRPSDefault); err != nil {
		utils.Error(fmt.Errorf("runtime provider RPS default invalid; keeping code/environment defaults: %w", err))
		candidate = fallback
		if fallbackErr := validateProviderRPSConfig(fallback); fallbackErr != nil {
			return fmt.Errorf("invalid fallback provider RPS configuration: %w", fallbackErr)
		}
	}
	if !validPositiveConfigInt(candidate.ProviderRPSTaskSlots) {
		utils.Error(fmt.Errorf("ENV PROVIDER_RPS_TASK_SLOTS invalid; keeping code/environment default"))
		candidate.ProviderRPSTaskSlots = fallback.ProviderRPSTaskSlots
	}
	if !validPositiveConfigInt(candidate.ConsumerDefaultClientWorkers) {
		utils.Error(fmt.Errorf("ENV CONSUMER_DEFAULT_CLIENT_WORKERS invalid; keeping code/environment default"))
		candidate.ConsumerDefaultClientWorkers = fallback.ConsumerDefaultClientWorkers
	}
	if !validPositiveConfigInt(candidate.ConsumerClientBufferSize) {
		utils.Error(fmt.Errorf("ENV CONSUMER_CLIENT_BUFFER_SIZE invalid; keeping code/environment default"))
		candidate.ConsumerClientBufferSize = fallback.ConsumerClientBufferSize
	}
	if !validPositiveConfigInt(candidate.GoMaxProcs) {
		utils.Error(fmt.Errorf("ENV GOMAXPROCS invalid; keeping code/environment default"))
		candidate.GoMaxProcs = fallback.GoMaxProcs
	}
	if err := validateWorkerOverrides(candidate.ConsumerClientWorkerOverrides, false); err != nil {
		utils.Error(fmt.Errorf("AppConfig CONSUMER_CLIENT_WORKER_OVERRIDES invalid; keeping code/environment default: %w", err))
		candidate.ConsumerClientWorkerOverrides = fallback.ConsumerClientWorkerOverrides
	}
	if err := validateWorkerOverrides(candidate.ConsumerChannelWorkerOverrides, true); err != nil {
		utils.Error(fmt.Errorf("AppConfig CONSUMER_CHANNEL_WORKER_OVERRIDES invalid; keeping code/environment default: %w", err))
		candidate.ConsumerChannelWorkerOverrides = fallback.ConsumerChannelWorkerOverrides
	}
	if candidate.SMSWorkers != "" && !validPositiveConfigInt(candidate.SMSWorkers) {
		utils.Error(fmt.Errorf("ENV SMS_WORKERS invalid; keeping code/environment default"))
		candidate.SMSWorkers = fallback.SMSWorkers
	}
	if candidate.WhatsAppWorkers != "" && !validPositiveConfigInt(candidate.WhatsAppWorkers) {
		utils.Error(fmt.Errorf("ENV WHATSAPP_WORKERS invalid; keeping code/environment default"))
		candidate.WhatsAppWorkers = fallback.WhatsAppWorkers
	}

	if err := validateAsyncWriteChannels(candidate.CommAsyncWriteChannels); err != nil {
		utils.Error(fmt.Errorf("AppConfig COMM_ASYNC_WRITE_CHANNELS invalid; keeping environment/default: %w", err))
		candidate.CommAsyncWriteChannels = fallback.CommAsyncWriteChannels
	}

	if err := validateStreamBudgets(candidate.CommWriteStreamBudgetOverrides); err != nil {
		utils.Error(fmt.Errorf("AppConfig COMM_WRITE_STREAM_BUDGET_OVERRIDES invalid; keeping environment/default: %w", err))
		candidate.CommWriteStreamBudgetOverrides = fallback.CommWriteStreamBudgetOverrides
	}

	Configs = candidate
	logResolvedRuntimeTuning(Configs, appConfigValues)
	utils.SetLogLevel(Configs.LogLevel)
	procs := positiveConfigInt(Configs.GoMaxProcs, 1)
	runtime.GOMAXPROCS(int(procs))
	return nil
}

func logResolvedRuntimeTuning(resolved models.Config, appValues map[string]string) {
	source := func(key, value string) string {
		if raw, ok := appValues[key]; ok && strings.TrimSpace(raw) != "" && value == strings.TrimSpace(raw) {
			return "appconfig"
		}
		return "env_or_default"
	}

	utils.Info(fmt.Sprintf(
		"runtime_tuning_resolved provider_overrides=%q[%s] provider_caps=%q[%s] provider_default=%s[%s] task_slots=%s[%s] default_workers=%s[%s] client_workers=%q[%s] channel_workers=%q[%s] buffer=%s[%s] gomaxprocs=%s[%s] log_level=%s[%s] async_writes=%q[%s] stream_budgets=%q[%s]",
		resolved.ProviderRPSOverrides, source("PROVIDER_RPS_OVERRIDES", resolved.ProviderRPSOverrides),
		resolved.ProviderRPSApprovedCaps, source("PROVIDER_RPS_APPROVED_CAPS", resolved.ProviderRPSApprovedCaps),
		resolved.ProviderRPSDefault, source("PROVIDER_RPS_DEFAULT", resolved.ProviderRPSDefault),
		resolved.ProviderRPSTaskSlots, source("PROVIDER_RPS_TASK_SLOTS", resolved.ProviderRPSTaskSlots),
		resolved.ConsumerDefaultClientWorkers, source("CONSUMER_DEFAULT_CLIENT_WORKERS", resolved.ConsumerDefaultClientWorkers),
		resolved.ConsumerClientWorkerOverrides, source("CONSUMER_CLIENT_WORKER_OVERRIDES", resolved.ConsumerClientWorkerOverrides),
		resolved.ConsumerChannelWorkerOverrides, source("CONSUMER_CHANNEL_WORKER_OVERRIDES", resolved.ConsumerChannelWorkerOverrides),
		resolved.ConsumerClientBufferSize, source("CONSUMER_CLIENT_BUFFER_SIZE", resolved.ConsumerClientBufferSize),
		resolved.GoMaxProcs, source("GOMAXPROCS", resolved.GoMaxProcs),
		resolved.LogLevel, source("LOG_LEVEL", resolved.LogLevel),
		resolved.CommAsyncWriteChannels, source("COMM_ASYNC_WRITE_CHANNELS", resolved.CommAsyncWriteChannels),
		resolved.CommWriteStreamBudgetOverrides, source("COMM_WRITE_STREAM_BUDGET_OVERRIDES", resolved.CommWriteStreamBudgetOverrides),
	))
}

// ApplyAppConfigValues overlays nonblank central tuning values onto a config.
func ApplyAppConfigValues(candidate *models.Config, values map[string]string) {
	apply := func(key string, dst *string) {
		if raw := values[key]; raw != "" {
			*dst = raw
			utils.Info("AppConfig applied " + key)
		} else if _, exists := values[key]; exists {
			utils.Info("AppConfig " + key + " blank; keeping fallback")
		}
	}
	apply("PROVIDER_RPS_OVERRIDES", &candidate.ProviderRPSOverrides)
	apply("CONSUMER_CLIENT_WORKER_OVERRIDES", &candidate.ConsumerClientWorkerOverrides)
	apply("CONSUMER_CHANNEL_WORKER_OVERRIDES", &candidate.ConsumerChannelWorkerOverrides)
	apply("LOG_LEVEL", &candidate.LogLevel)
	apply("COMM_ASYNC_WRITE_CHANNELS", &candidate.CommAsyncWriteChannels)
	apply("COMM_WRITE_STREAM_BUDGET_OVERRIDES", &candidate.CommWriteStreamBudgetOverrides)
}

func validPositiveConfigInt(raw string) bool {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	return err == nil && value > 0
}

func validateWorkerOverrides(raw string, channelScoped bool) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		want := 2
		if channelScoped {
			want = 3
		}
		if len(parts) != want || strings.TrimSpace(parts[0]) == "" || (channelScoped && strings.TrimSpace(parts[1]) == "") || !validPositiveConfigInt(parts[len(parts)-1]) {
			return fmt.Errorf("invalid worker override %q", entry)
		}
	}
	return nil
}

func validateAsyncWriteChannels(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	for _, entry := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("invalid async-write channel %q", entry)
		}
	}

	return nil
}

func validateStreamBudgets(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	for _, entry := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || !validPositiveConfigInt(parts[2]) {
			return fmt.Errorf("invalid stream budget %q", entry)
		}
	}

	return nil
}

func validateProviderRPSConfig(configs models.Config) error {
	if err := ratelimit.ValidateOverridesAgainstCaps(configs.ProviderRPSOverrides, configs.ProviderRPSApprovedCaps); err != nil {
		return err
	}
	return validateProviderRPSDefault(configs.ProviderRPSDefault)
}

func validateProviderRPSDefault(raw string) error {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("invalid PROVIDER_RPS_DEFAULT %q", raw)
	}
	return nil
}
