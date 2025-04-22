package config

import (
	"fmt"
	"os"
	"reflect"

	"github.com/joho/godotenv"
	"github.com/wecredit/communication-sdk/sdk/internal/models"
)

// Create an instance of Config
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

	// Connect Analytics DB
	// err := database.ConnectDB(database.Analytics, Configs)
	// if err != nil {
	// 	return fmt.Errorf("failed to initialize Analytics database: %v", err)
	// }
	// utils.Info("Analytics Database connection pool initialized successfully.")

	// // Connect Tech DB
	// err = database.ConnectDB(database.Tech, Configs)
	// if err != nil {
	// 	return fmt.Errorf("failed to initialize Tech database: %v", err)
	// }
	// utils.Info("Tech Database connection pool initialized successfully.")

	// Configure Queue client
	// err = queue.InitClient(Configs.AzureCallbackServiceBusConnectionString, true)
	// if err != nil {
	// 	return fmt.Errorf("failed to initialize Azure Service Bus client: %v", err)
	// }

	// // Initialize Redis Connection
	// _, err = redis.GetRedisClient(Configs.RedisAddress, Configs.RedisPassword)
	// if err != nil {
	// 	utils.Error(fmt.Errorf("failed to initialize redis connection"))
	// }

	// Initiate Default quueue client
	// fmt.Println(Configs.AzureServiceBusConnectionString)
	// queue.GetClient(Configs.AzureServiceBusConnectionString)

	// Initialte client for callback queue
	// queue.GetCallbackClient(Configs.AzureCallbackServiceBusConnectionString)

	return nil
}
