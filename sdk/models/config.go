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
	DbServerTech   string `envconfig:"DB_SERVER_TECH"`
	DbPortTech     string `envconfig:"DB_PORT_TECH"`
	DbUserTech     string `envconfig:"DB_USER_TECH"`
	DbPasswordTech string `envconfig:"DB_PASSWORD_TECH"`
	DbNameTech     string `envconfig:"DB_NAME_TECH"`

	// Azure Queue Details
	QueueConnectionString string `envconfig:"AZURE_SERVICEBUS_CONNECTION_STRING"`
	QueueTopicName        string `envconfig:"AZURE_TOPIC_NAME"`
	QueueSubscriptionName string `envconfig:"AZURE_DB_SUBSCRIPTION"`
	BasicAuthApiUrl       string `envconfig:"BASIC_AUTH_API_URL"`

	// AWS Credentials
	AWSRegion   string `envconfig:"AWS_REGION"`
	AwsSnsArn   string `envconfig:"AWS_COMM_TOPIC_ARN"`
	AwsQueueUrl string `envconfig:"AWS_QUEUE_URL"`

	// Auth Table Variables
	BasicAuthTableName string `envconfig:"BASIC_AUTH_TABLE"`

	// SDK Tables
	SdkWhatsappInputTable string `envconfig:"SDK_WHATSAPP_INPUT_TABLE"`
	WhatsappOutputTable   string `envconfig:"WHATSAPP_OUTPUT_TABLE"`

	SdkRcsInputTable string `envconfig:"SDK_RCS_INPUT_TABLE"`
	RcsOutputTable   string `envconfig:"RCS_OUTPUT_TABLE"`

	SdkSmsInputTable string `envconfig:"SDK_SMS_INPUT_TABLE"`
	SmsOutputTable   string `envconfig:"SMS_OUTPUT_TABLE"`

	VendorTable          string `envconfig:"VENDORS_TABLE"`
	ClientsTable         string `envconfig:"CLIENTS_TABLE"`
	TemplateDetailsTable string `envconfig:"TEMPLATE_TABLE"`

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
	CreditSeaSinchSmsApiAppID    string `envconfig:"CREDITSEA_SINCH_SMS_API_APP_ID"`
	CreditSeaSinchSmsApiUserName string `envconfig:"CREDITSEA_SINCH_SMS_API_USERNAME"`
	CreditSeaSinchSmsApiPassword string `envconfig:"CREDITSEA_SINCH_SMS_API_PASSWORD"`
	CreditSeaSinchSmsApiSender   string `envconfig:"CREDITSEA_SINCH_SMS_API_SENDER"`
}
