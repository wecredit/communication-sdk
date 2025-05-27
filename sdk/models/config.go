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
	AWSRegion string `envconfig:"AWS_REGION"`
	AwsSnsArn string `envconfig:"AWS_COMM_TOPIC_ARN"`
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

	// RCS Tables
	RcsTemplateAppIdTable string `envconfig:"RCS_TEMPLATE_APP_ID_TABLE"`

	// Sinch API Variables
	SinchTokenApiUrl   string `envconfig:"SINCH_GENERATE_TOKEN_API_URL"`
	SinchMessageApiUrl string `envconfig:"SINCH_SEND_MESSAGE_API_URL"`
	SinchGrantType     string `envconfig:"SINCH_API_GRANT_TYPE"`
	SinchClientId      string `envconfig:"SINCH_API_CLIENT_ID"`
	SinchUserName      string `envconfig:"SINCH_API_USERNAME"`
	SinchPassword      string `envconfig:"SINCH_API_PASSWORD"`
	SinchCallbackURL   string `envconfig:"SINCH_WP_CALLBACK_URL"`
	SinchRcsApiUrl     string `envconfig:"SINCH_RCS_API_URL"`

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
	SinchSmsDltContentId string `envconfig:"SINCH_SMS_API_DLTCONTENTID"`
	SinchSmsApiSender    string `envconfig:"SINCH_SMS_API_SENDER"`
	SinchSmsApiUrl       string `envconfig:"SINCH_SMS_API_URL"`
}
