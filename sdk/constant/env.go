package env

const (
	DB_SERVER_ANALYTICS   = "13.127.10.86"
	DB_PORT_ANALYTICS     = "1433"
	DB_USER_ANALYTICS     = "Amartya"
	DB_PASSWORD_ANALYTICS = "WeCred!T@2302$"
	DB_NAME_ANALYTICS     = "master"

	DB_SERVER_TECH   = "13.127.97.26"
	DB_PORT_TECH     = "1433"
	DB_USER_TECH     = "WCAppUser"
	DB_PASSWORD_TECH = "WeCred!TaPP@2025"
	DB_NAME_TECH     = "communication"

	// Azure Queue Details
	AZURE_SERVICEBUS_CONNECTION_STRING = "Endpoint=sb://communication-service-engine.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=Zogu1EUScN51b9nd8clFiRijdxdspIiFd+ASbED8jkY="
	AZURE_TOPIC_NAME                   = "communication-uat"
	AZURE_DB_SUBSCRIPTION              = "priority"
	BASIC_AUTH_API_URL                 = "http://communication-sdk.wc-prod-services:8080/clients/validate-client"
	// BASIC_AUTH_API_URL                 = "http://localhost:8080/clients/validate-client"

	AWS_REGION         = "ap-south-1"
	AWS_COMM_TOPIC_ARN = "arn:aws:sns:ap-south-1:717840664658:comm-sdk"

	// Times API Details
	TIMES_WP_API_URL   = "https://wecredit1.timespanel.in/wa/v2/messages/send"
	TIMES_WP_API_TOKEN = "9a0ca0c782680cc6348da75f6fe97f060ee0c52ec742be2186"

	// Sinch API Details
	SINCH_GENERATE_TOKEN_API_URL = "https://auth.aclwhatsapp.com/realms/ipmessaging/protocol/openid-connect/token"
	SINCH_SEND_MESSAGE_API_URL   = "https://api.aclwhatsapp.com/pull-platform-receiver/v2/wa/messages"
	SINCH_API_GRANT_TYPE         = "password"
	SINCH_API_CLIENT_ID          = "ipmessaging-client"
	SINCH_API_USERNAME           = "wecreditpd"
	SINCH_API_PASSWORD           = "Sinch@8655685383"
	SINCH_WP_CALLBACK_URL        = "https://sinch-whatsapp-callback-api-h0a0hjafgvbjb7cb.centralindia-01.azurewebsites.net/api/v1/sinch-whatsapp/callback"
	SINCH_RCS_API_URL            = "https://convapi.aclwhatsapp.com/v1/projects/"
	SINCH_SMS_API_URL            = "https://push3.aclgateway.com/v1/enterprises/messages.json"

	// Log Level
	LOG_LEVEL = "DEBUG"
)
