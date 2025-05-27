package config

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/service/sns"
	env "github.com/wecredit/communication-sdk/sdk/constant"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/models"
)

// Create an instance of Config
var SdkConfigs models.Config

func LoadSDKConfigs() (*sns.SNS, error) {
	// Use reflection to set the struct fields with environment variables
	val := reflect.ValueOf(&SdkConfigs).Elem() // Pass a pointer to the struct
	typ := reflect.TypeOf(SdkConfigs)          // Use the struct type (not the pointer)

	// Map of envconfig tags to their constant values
	envVars := map[string]string{
		// "DB_SERVER_ANALYTICS":                env.DB_SERVER_ANALYTICS,
		// "DB_PORT_ANALYTICS":                  env.DB_PORT_ANALYTICS,
		// "DB_USER_ANALYTICS":                  env.DB_USER_ANALYTICS,
		// "DB_PASSWORD_ANALYTICS":              env.DB_PASSWORD_ANALYTICS,
		// "DB_NAME_ANALYTICS":                  env.DB_NAME_ANALYTICS,
		// "DB_SERVER_TECH":                     env.DB_SERVER_TECH,
		// "DB_PORT_TECH":                       env.DB_PORT_TECH,
		// "DB_USER_TECH":                       env.DB_USER_TECH,
		// "DB_PASSWORD_TECH":                   env.DB_PASSWORD_TECH,
		// "DB_NAME_TECH":                       env.DB_NAME_TECH,
		"AZURE_SERVICEBUS_CONNECTION_STRING": env.AZURE_SERVICEBUS_CONNECTION_STRING,
		"AZURE_TOPIC_NAME":                   env.AZURE_TOPIC_NAME,
		"BASIC_AUTH_API_URL":                 env.BASIC_AUTH_API_URL,
		"AWS_REGION":                         env.AWS_REGION,
		"AWS_COMM_TOPIC_ARN":                 env.AWS_COMM_TOPIC_ARN,
		// "AZURE_DB_SUBSCRIPTION":              env.AZURE_DB_SUBSCRIPTION,
		// "TIMES_WP_API_URL":                   env.TIMES_WP_API_URL,
		// "TIMES_WP_API_TOKEN":                 env.TIMES_WP_API_TOKEN,
		// "SINCH_GENERATE_TOKEN_API_URL":       env.SINCH_GENERATE_TOKEN_API_URL,
		// "SINCH_SEND_MESSAGE_API_URL":         env.SINCH_SEND_MESSAGE_API_URL,
		// "SINCH_API_GRANT_TYPE":               env.SINCH_API_GRANT_TYPE,
		// "SINCH_API_CLIENT_ID":                env.SINCH_API_CLIENT_ID,
		// "SINCH_API_USERNAME":                 env.SINCH_API_USERNAME,
		// "SINCH_API_PASSWORD":                 env.SINCH_API_PASSWORD,
		// "SINCH_WP_CALLBACK_URL":              env.SINCH_WP_CALLBACK_URL,
		// "SINCH_RCS_API_URL":                  env.SINCH_RCS_API_URL,
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
	client, err := queue.GetSdkSnsClient(SdkConfigs.AWSRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SDK Client: %v", err)
	}

	return client, nil
}
