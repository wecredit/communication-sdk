package env

const (
	// Azure Queue Details
	AZURE_SERVICEBUS_CONNECTION_STRING = "Endpoint=sb://communication-service-engine.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=Zogu1EUScN51b9nd8clFiRijdxdspIiFd+ASbED8jkY="
	AZURE_TOPIC_NAME                   = "communication-uat"
	AZURE_DB_SUBSCRIPTION              = "communication-sub"

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
	SINCH_RCS_API_URL            = "https://convapi.aclwhatsapp.com/v1/projects/2a62221d-e5ca-498e-8aef-5cf025d0eba9/messages:send"

	// Log Level
	LOG_LEVEL = "DEBUG"
)
