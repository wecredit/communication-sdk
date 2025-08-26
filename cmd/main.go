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
	port := os.Getenv("CONSUMER_SERVER_PORT")
	if port == "" {
		port = "8080" // default port
	}
	server.StartConsumer(port)
}
