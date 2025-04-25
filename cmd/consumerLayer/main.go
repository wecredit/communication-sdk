package main

import (
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/server"
)

func init() {
	// Load configs
	config.LoadConfigs()

}

func main() {
	// Start consumer server
	server.StartConsumer("8080")
}
