package models

type Config struct {
	Port         string `envconfig:"API_SERVER_PORT"`
	ConsumerPort string `envconfig:"CONSUMER_SERVER_PORT"`

	// Analytical DB variables
	DbServerAnalytical   string `envconfig:"DB_SERVER_ANALYTICS"`
	DbPortAnalytical     string `envconfig:"DB_PORT_ANALYTICS"`
	DbUserAnalytical     string `envconfig:"DB_USER_ANALYTICS"`
	DbPasswordAnalytical string `envconfig:"DB_PASSWORD_ANALYTICS"`
	DbNameAnalytical     string `envconfig:"DB_NAME_ANALYTICS"`

	// Tech DB variables

	// DbServerTech   string `envconfig:"DB_SERVER_TECH"` // NOT IN USE AS OF NOW
	DbServerTechRead  string `envconfig:"DB_SERVER_TECH_READ"`
	DbServerTechWrite string `envconfig:"DB_SERVER_TECH_WRITE"`
	DbPortTech        string `envconfig:"DB_PORT_TECH"`
	DbUserTech        string `envconfig:"DB_USER_TECH"`
	DbPasswordTech    string `envconfig:"DB_PASSWORD_TECH"`
	DbNameTech        string `envconfig:"DB_NAME_TECH"`
	DbMaxOpenConns    string `envconfig:"DB_MAX_OPEN_CONNS"`
	DbMaxIdleConns    string `envconfig:"DB_MAX_IDLE_CONNS"`
	DbConnMaxLifetime string `envconfig:"DB_CONN_MAX_LIFETIME_MINUTES"`

	// Marketing SQL Server (same DB as CommMarketingInput / dbo.CommDispatchTracking).
	DbServerMarketing         string `envconfig:"DB_SERVER_MARKETING"`
	DbPortMarketing           string `envconfig:"DB_PORT_MARKETING"`
	DbUserMarketing           string `envconfig:"DB_USER_MARKETING"`
	DbPasswordMarketing       string `envconfig:"DB_PASSWORD_MARKETING"`
	DbNameMarketing           string `envconfig:"DB_NAME_MARKETING"`
	CommDispatchTrackingTable string `envconfig:"COMM_DISPATCH_TRACKING_TABLE" default:"dbo.CommDispatchTracking"`
	CommMarketingInputTable   string `envconfig:"COMM_MARKETING_INPUT_TABLE_NAME" default:"dbo.CommMarketingInput"`

	// Aws Queue Details
	QueueConnectionString string `envconfig:"AZURE_SERVICEBUS_CONNECTION_STRING"`
	QueueTopicName        string `envconfig:"AZURE_TOPIC_NAME"`
	QueueSubscriptionName string `envconfig:"AZURE_DB_SUBSCRIPTION"`
	BasicAuthApiUrl       string `envconfig:"BASIC_AUTH_API_URL"`

	// AWS Credentials
	AWSRegion string `envconfig:"AWS_REGION"`
	AwsSnsArn string `envconfig:"AWS_COMM_TOPIC_ARN"`
	// WeCredit SMS: prefer SQS-direct (plan A). When set, SDK Send skips SNS for wecredit+SMS.
	AwsWeCreditSmsQueueUrl string `envconfig:"AWS_WECREDIT_SMS_QUEUE_URL"`
	// Deprecated for WeCredit SMS isolation — kept only as optional SNS fallback if queue URL is empty.
	AwsWeCreditSmsTopicArn string `envconfig:"AWS_WECREDIT_SMS_TOPIC_ARN"`
	AwsQueueUrl            string `envconfig:"AWS_QUEUE_URL"`
	AwsErrorQueueUrl       string `envconfig:"AWS_COMM_ERROR_QUEUE_URL"`

	// Redis Credentials
	RedisAddress      string `envconfig:"REDIS_ADDRESS"`
	RedisPassword     string `envconfig:"REDIS_PASSWORD"`
	RedisMapKey       string `envconfig:"REDIS_MAP_KEY"`
	CommIdempotentKey string `envconfig:"COMM_IDEMPOTENT_KEY"`

	// ConfigurationVersion is durable cache state. Redis pub/sub is only the
	// fast notification path; polling this version recovers missed messages.
	Environment                     string `envconfig:"ENVIRONMENT" default:"dev"`
	ConfigurationVersionTable       string `envconfig:"CONFIGURATION_VERSION_TABLE" default:"ConfigurationVersion"`
	CacheVersionPollIntervalSeconds string `envconfig:"CACHE_VERSION_POLL_INTERVAL_SECONDS" default:"90"`
	CacheReloadMinIntervalSeconds   string `envconfig:"CACHE_RELOAD_MIN_INTERVAL_SECONDS" default:"60"`
	CommAdminUsername               string `envconfig:"COMM_ADMIN_USERNAME"`
	CommAdminPassword               string `envconfig:"COMM_ADMIN_PASSWORD"`

	CreditSeaWhatsappCurrentCount string `envconfig:"CREDITSEA_WHATSAPP_CURRENT_COUNT"`
	CreditSeaWhatsappMaxCount     string `envconfig:"CREDITSEA_WHATSAPP_MAX_COUNT"`

	// Auth Table Variables
	BasicAuthTableName string `envconfig:"BASIC_AUTH_TABLE"`

	// SDK Tables
	SdkWhatsappInputTable string `envconfig:"SDK_WHATSAPP_INPUT_TABLE"`
	WhatsappOutputTable   string `envconfig:"WHATSAPP_OUTPUT_TABLE"`

	SdkRcsInputTable string `envconfig:"SDK_RCS_INPUT_TABLE"`
	RcsOutputTable   string `envconfig:"RCS_OUTPUT_TABLE"`

	SdkSmsInputTable string `envconfig:"SDK_SMS_INPUT_TABLE"`
	SmsOutputTable   string `envconfig:"SMS_OUTPUT_TABLE"`

	SdkEmailInputTable string `envconfig:"SDK_EMAIL_INPUT_TABLE"`
	EmailOutputTable   string `envconfig:"EMAIL_OUTPUT_TABLE"`

	VendorTable          string `envconfig:"VENDORS_TABLE"`
	ClientsTable         string `envconfig:"CLIENTS_TABLE"`
	TemplateDetailsTable string `envconfig:"TEMPLATE_TABLE"`
	LenderStagesTable    string `envconfig:"LENDER_STAGES_TABLE_NAME" default:"LendersStages"`
	TemplateStageTable   string `envconfig:"TEMPLATE_STAGE_TABLE_NAME" default:"TemplateStage"`

	CommAuditTable string `envconfig:"COMM_AUDIT_TABLE"`

	// RCS Tables
	RcsTemplateAppIdTable string `envconfig:"RCS_TEMPLATE_APP_ID_TABLE"`

	// Sinch API Variables
	SinchWhatsappTokenApiUrl   string `envconfig:"SINCH_GENERATE_TOKEN_API_URL"`
	SinchWhatsappMessageApiUrl string `envconfig:"SINCH_SEND_WHATSAPP_MESSAGE_API_URL"`
	SinchWhatsappGrantType     string `envconfig:"SINCH_API_GRANT_TYPE"`
	SinchWhatsappClientId      string `envconfig:"SINCH_API_CLIENT_ID"`
	SinchWhatsappUserName      string `envconfig:"SINCH_API_USERNAME"`
	SinchWhatsappPassword      string `envconfig:"SINCH_API_PASSWORD"`
	SinchWhatsappCallbackURL   string `envconfig:"SINCH_WP_CALLBACK_URL"`
	SinchRcsApiUrl             string `envconfig:"SINCH_RCS_API_URL"`

	// Sinch Whatsapp CreditSea  Variables
	CreditSeaSinchWhatsappUsername string `envconfig:"SINCH_CREDITSEA_API_USERNAME"`
	CreditSeaSinchWhatsappPassword string `envconfig:"SINCH_CREDITSEA_API_PASSWORD"`

	// Times API Details
	TimesWpApiUrl   string `envconfig:"TIMES_WP_API_URL"`
	TimesWpAPIToken string `envconfig:"TIMES_WP_API_TOKEN"`

	// Times SMS API Variables
	TimesSmsApiUserName  string `envconfig:"TIMES_SMS_API_USERNAME"`
	TimesSmsApiPassword  string `envconfig:"TIMES_SMS_API_PASSWORD"`
	TimesSmsDltContentId string `envconfig:"TIMES_SMS_API_DLTCONTENTID"`
	TimesSmsApiSender    string `envconfig:"TIMES_SMS_API_SENDER"`
	TimesSmsApiUrl       string `envconfig:"TIMES_SMS_API_URL"`

	// Sinch SMS API Variables
	SinchSmsApiAppID     string `envconfig:"SINCH_SMS_API_APP_ID"`
	SinchSmsApiUserName  string `envconfig:"SINCH_SMS_API_USERNAME"`
	SinchSmsApiPassword  string `envconfig:"SINCH_SMS_API_PASSWORD"`
	SinchSmsApiSender    string `envconfig:"SINCH_SMS_API_SENDER"`
	SinchSmsDltContentId string `envconfig:"SINCH_SMS_API_DLTCONTENTID"`
	SinchSmsApiUrl       string `envconfig:"SINCH_SMS_API_URL"`

	// CreditSea Sinch SMS API Variables
	CreditSeaSinchSmsApiAppID     string `envconfig:"CREDITSEA_SINCH_SMS_API_APP_ID"`
	CreditSeaSinchSmsApiUserName  string `envconfig:"CREDITSEA_SINCH_SMS_API_USERNAME"`
	CreditSeaSinchSmsApiPassword  string `envconfig:"CREDITSEA_SINCH_SMS_API_PASSWORD"`
	CreditSeaSinchSmsApiSender    string `envconfig:"CREDITSEA_SINCH_SMS_API_SENDER"`
	ConsumerDefaultClientWorkers  string `envconfig:"CONSUMER_DEFAULT_CLIENT_WORKERS" default:"5"`
	ConsumerClientWorkerOverrides string `envconfig:"CONSUMER_CLIENT_WORKER_OVERRIDES"`
	ConsumerClientBufferSize      string `envconfig:"CONSUMER_CLIENT_BUFFER_SIZE" default:"100"`

	// Per-provider SMS outbound rate limits (token bucket; no external deps).
	// Overrides format: vendor:client:rps or vendor:rps (comma-separated).
	ProviderRPSDefault   string `envconfig:"PROVIDER_RPS_DEFAULT" default:"50"`
	ProviderRPSOverrides string `envconfig:"PROVIDER_RPS_OVERRIDES"`

	// Sinch Email API Variables
	SinchEmailApiUrl   string `envconfig:"SINCH_EMAIL_API_URL"`
	SinchEmailApiToken string `envconfig:"SINCH_EMAIL_API_TOKEN"`

	// ZapCash Pinnacle Whatsapp Variables
	PinnacleZapcashWhatsappApiKey        string `envconfig:"PINNACLE_ZAPCASH_WHATSAPP_API_KEY"`
	PinnacleZapcashWhatsappMessageApiUrl string `envconfig:"PINNACLE_ZAPCASH_WHATSAPP_MESSAGE_API_URL"`
	PinnacleZapcashWabaId                string `envconfig:"PINNACLE_ZAPCASH_WABA_ID"`

	// ZapCash Pinnacle RCS Variables
	PinnacleZapcashRcsApiUrl             string `envconfig:"PINNACLE_ZAPCASH_RCS_API_URL"`
	PinnacleZapcashRcsApiKey             string `envconfig:"PINNACLE_ZAPCASH_RCS_API_KEY"`
	PinnacleZapcashRcsTransactionalBotId string `envconfig:"PINNACLE_ZAPCASH_RCS_TRANSACTIONAL_BOT_ID"`
	PinnacleZapcashRcsPromotionalBotId   string `envconfig:"PINNACLE_ZAPCASH_RCS_PROMOTIONAL_BOT_ID"`
	PinnacleZapcashRcsTtl                string `envconfig:"PINNACLE_ZAPCASH_RCS_TTL"`

	// Pinnacle SMS API
	PinnacleSmsApiUrl      string `envconfig:"PINNACLE_SMS_API_URL"`
	PinnacleSmsAccessKey   string `envconfig:"PINNACLE_SMS_ACCESSKEY"`
	PinnacleSmsDltEntityId string `envconfig:"PINNACLE_SMS_DLT_ENTITY_ID"`
}
