package main

import (
	"os"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/server"
)

func init() {
	// Load configs
	config.LoadConfigs()

}

func main() {
	// Start consumer server
	server.StartConsumer(os.Getenv("CONSUMER_SERVER_PORT"))
}
