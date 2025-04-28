package main

import (
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/server"
)

func init() {
	// // Load configurations
	// _, err := config.LoadConfigs()
	// if err != nil {
	// 	utils.Error(err)
	// }
	fmt.Println("Configurations loaded successfully.")
}

func main() {
	// Consume queued data
	server.Start()
}
