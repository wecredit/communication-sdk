package sdk

import (
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

type CommSdkClient struct {
	Username string
	Password string
	isAuthed bool
}

func NewSdkClient(username, password string) (*CommSdkClient, error) {
	err := config.LoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load configs: %v", err)
	}
	utils.Debug("Configurations loaded successfully.")

	// // Validate user credentials
	// if !validateUser(username, password) {
	// 	return nil, fmt.Errorf("invalid credentials")
	// }

	// DB connection
	// db, err := gorm.Open(sqlserver.Open(dsnAnalytics), &gorm.Config{})
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to connect to Analytics DB: %w", err)
	// }
	// utils.Debug("DB connection successful.")

	return &CommSdkClient{
		Username: username,
		Password: password,
		isAuthed: true,
	}, nil
}
