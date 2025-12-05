package server

import (
	"log"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/cron"
	"github.com/wecredit/communication-sdk/health"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/handlers"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
)

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("Error getting IP address: %v", err)
		return "unknown"
	}

	for _, addr := range addrs {
		// Skip loopback and check for IPNet type
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			// Return the first non-loopback IPv4 address
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "not found"
}

func StartConsumer(port string) {
	go services.ConsumerService(10, config.Configs.AwsQueueUrl)
	go cron.StartMidnightResetCron()

	log.Printf("Starting Consumer Server on port %s", port)

	// Set up Gin router
	r := gin.Default()

	r.GET("/health", health.HealthCheckHandler(port))

	vendorHandler := handlers.NewVendorHandler(apiServices.NewVendorService(database.DBtechWrite)) // Create handler for vendors passing them database object
	vendors := r.Group("/vendors")
	{
		vendors.GET("/", vendorHandler.GetVendors) // endpoint:- /vendors; filter: ?channel=WHATSAPP
		vendors.POST("/add-vendor", vendorHandler.AddVendor)
		vendors.PUT("/:name/:channel", vendorHandler.UpdateVendorByNameAndChannel)
		vendors.GET("/id/:id", vendorHandler.GetVendorByID) // endpoint:- /vendors/{id};
		vendors.DELETE("/id/:id", vendorHandler.DeleteVendor)
	}

	clientHandler := handlers.NewClientHandler(apiServices.NewClientService(database.DBtechWrite)) // Create handler for clients passing them database object
	clients := r.Group("/clients")
	{
		clients.GET("/", clientHandler.GetClients)
		clients.POST("/add-client", clientHandler.AddClient)
		clients.PUT("/:name/:channel", clientHandler.UpdateClientByNameAndChannel)
		clients.GET("/id/:id", clientHandler.GetClientByID)
		clients.DELETE("/id/:id", clientHandler.DeleteClient)
		clients.POST("/validate-client", clientHandler.ValidateClient)
	}

	templateHandler := handlers.NewTemplateHandler(apiServices.NewTemplateService(database.DBtechWrite))
	templates := r.Group("/templates")
	{
		templates.GET("/", templateHandler.GetTemplates)
		templates.POST("/add-template", templateHandler.AddTemplate)
		templates.PUT("/id/:id", templateHandler.UpdateTemplateById)
		templates.GET("/id/:id", templateHandler.GetTemplateByID)
		templates.DELETE("/id/:id", templateHandler.DeleteTemplate)
	}

	// if err := r.Run(":" + port); err != nil {
	if err := r.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
