package sdkConfig

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/wecredit/communication-sdk/internal/redis"
	env "github.com/wecredit/communication-sdk/sdk/constant"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// Create an instance of Config
var SdkConfigs models.Config

func LoadSDKConfigs() (*sns.SNS, error) {
	// Use reflection to set the struct fields with environment variables
	val := reflect.ValueOf(&SdkConfigs).Elem() // Pass a pointer to the struct
	typ := reflect.TypeOf(SdkConfigs)          // Use the struct type (not the pointer)

	// Map of envconfig tags to their constant values
	envVars := map[string]string{
		"AWS_REGION":     env.AWS_REGION,
		"REDIS_ADDRESS":  env.REDIS_ADDRESS,
		"REDIS_PASSWORD": env.REDIS_PASSWORD,
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

	// Initialize Redis Connection
	_, err := redis.GetRedisClient(SdkConfigs.RedisAddress, SdkConfigs.RedisPassword)
	if err != nil {
		utils.Error(fmt.Errorf("failed to initialize redis connection"))
	}

	// Initiate Default quueue client
	client, err := queue.GetSdkSnsClient(SdkConfigs.AWSRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SDK Client: %v", err)
	}

	return client, nil
}
