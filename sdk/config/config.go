package config

import (
	"reflect"

	env "github.com/wecredit/communication-sdk/sdk/constant"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/models"
)

// Create an instance of Config
var Configs models.Config

func LoadConfigs() error {
	// Use reflection to set the struct fields with environment variables
	val := reflect.ValueOf(&Configs).Elem() // Pass a pointer to the struct
	typ := reflect.TypeOf(Configs)          // Use the struct type (not the pointer)

	// Map of envconfig tags to their constant values
	envVars := map[string]string{
		"AZURE_SERVICEBUS_CONNECTION_STRING": env.AZURE_SERVICEBUS_CONNECTION_STRING,
		"AZURE_TOPIC_NAME":                   env.AZURE_TOPIC_NAME,
		"AZURE_DB_SUBSCRIPTION":              env.AZURE_DB_SUBSCRIPTION,
		"TIMES_WP_API_URL":                   env.TIMES_WP_API_URL,
		"TIMES_WP_API_TOKEN":                 env.TIMES_WP_API_TOKEN,
		"SINCH_GENERATE_TOKEN_API_URL":       env.SINCH_GENERATE_TOKEN_API_URL,
		"SINCH_SEND_MESSAGE_API_URL":         env.SINCH_SEND_MESSAGE_API_URL,
		"SINCH_API_GRANT_TYPE":               env.SINCH_API_GRANT_TYPE,
		"SINCH_API_CLIENT_ID":                env.SINCH_API_CLIENT_ID,
		"SINCH_API_USERNAME":                 env.SINCH_API_USERNAME,
		"SINCH_API_PASSWORD":                 env.SINCH_API_PASSWORD,
		"SINCH_WP_CALLBACK_URL":              env.SINCH_WP_CALLBACK_URL,
		"SINCH_RCS_API_URL":                  env.SINCH_RCS_API_URL,
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		envVar := typ.Field(i).Tag.Get("envconfig")

		if value, exists := envVars[envVar]; exists {
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

	// Initiate Default quueue client
	queue.GetClient(Configs.QueueConnectionString)

	return nil
}
