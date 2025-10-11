package config

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
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
