package server

import (
	"log"
	"net/http"

	"github.com/wecredit/communication-sdk/sdk/config"
)

func Start() {
	// Create a new ServeMux
	mux := http.NewServeMux()

	


	// Start the server
	port := config.Configs.Port
	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
