package main

import (
	"fmt"
	"os"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/server"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func init() {
	// Load configs
	if err := config.LoadConfigs(); err != nil {
		utils.Error(fmt.Errorf("failed to load configs: %v", err))
	}

	cache.LoadConsumerDataIntoCache(config.Configs)
}

func main() {
	// Start consumer server
	port := os.Getenv("CONSUMER_SERVER_PORT")
	if port == "" {
		port = "8080" // default port
	}
	server.StartConsumer(port)
}
