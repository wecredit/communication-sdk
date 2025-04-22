package server

import (
	"log"
	"net/http"

	"dev.azure.com/wctec/communication-engine/sdk/config"
	services "dev.azure.com/wctec/communication-engine/sdk/internal/services/consumerServices"
)

func StartConsumer(port string) {
	mux := http.NewServeMux()

	// go services.SendRCSMessage()

	go services.ConsumerService(10, config.Configs.AzureTopicName, config.Configs.AzureTopicSubscription)

	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
