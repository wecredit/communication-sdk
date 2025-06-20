package main

import (
	"os"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/server"
	"github.com/wecredit/communication-sdk/pkg/cache"
)

func init() {
	// Load configs
	config.LoadConfigs()
	cache.LoadConsumerDataIntoCache(config.Configs)
}

func main() {
	// Start consumer server
	server.StartConsumer(os.Getenv("CONSUMER_SERVER_PORT"))
}
