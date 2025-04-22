package server

import (
	"log"
	"net/http"

	"github.com/wecredit/communication-sdk/sdk/config"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/consumerServices"
)

func StartConsumer(port string) {
	mux := http.NewServeMux()

	// go services.SendRCSMessage()

	go services.ConsumerService(10, config.Configs.AzureTopicName, config.Configs.AzureTopicSubscription)

	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
