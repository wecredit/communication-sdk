package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/internal/database"
	internalredis "github.com/wecredit/communication-sdk/internal/redis"
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

	cacheOptions, err := configurationcache.OptionsFromConfig(config.Configs)
	if err != nil {
		log.Fatalf("invalid configuration cache settings: %v", err)
	}
	if _, err := configurationcache.StartController(
		context.Background(),
		database.DBtechRead,
		internalredis.RDB,
		cacheOptions,
	); err != nil {
		log.Fatalf("failed to start configuration cache controller: %v", err)
	}
}

func main() {
	// Start consumer server
	port := os.Getenv("CONSUMER_SERVER_PORT")
	if port == "" {
		port = "8080" // default port
	}
	server.StartConsumer(port)
}
