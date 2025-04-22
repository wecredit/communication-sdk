package server

import (
	"log"
	"net/http"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/handlers"
)

// Start the server
func StartServer() {
	// Create a new ServeMux
	mux := http.NewServeMux()

	apiPrefix := "/api/v1/comms"

	mux.Handle(apiPrefix+"/queue-insertion", http.HandlerFunc(handlers.HandleCommApi))

	// Start the server
	port := config.Configs.Port
	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
