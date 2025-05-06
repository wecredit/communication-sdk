package server

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/internal/handlers"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/consumerServices"
)

func StartConsumer(port string) {
	go services.ConsumerService(10, config.Configs.QueueTopicName, config.Configs.QueueSubscriptionName)

	// Set up Gin router
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "consumer API is running",
		})
	})

	vendorHandler := handlers.NewVendorHandler(services.NewVendorService(database.DBtech)) // Create handler for vendors passing them database object
	vendors := r.Group("/vendors")
	{
		vendors.GET("/", vendorHandler.GetVendors) // endpoint:- /vendors; filter: ?channel=WHATSAPP
		vendors.POST("/add-vendor", vendorHandler.AddVendor)
		vendors.PUT("/:name/:channel", vendorHandler.UpdateVendorByNameAndChannel)
		vendors.GET("/id/:id", vendorHandler.GetVendorByID) // endpoint:- /vendors/{id};
		vendors.DELETE("/id/:id", vendorHandler.DeleteVendor)
	}

	clientHandler := handlers.NewClientHandler(services.NewClientService(database.DBtech)) // Create handler for vendors passing them database object
	clients := r.Group("/clients")
	{
		clients.GET("/", clientHandler.GetClients)
		clients.POST("/add-client", clientHandler.AddClient)
		clients.PUT("/:name/:channel", clientHandler.UpdateClientByNameAndChannel)
		clients.GET("/id/:id", clientHandler.GetClientByID)
		clients.DELETE("/id/:id", clientHandler.DeleteClient)
		clients.POST("/validate-client", clientHandler.ValidateClient)
	}

	templateHandler := handlers.NewTemplateHandler(services.NewTemplateService(database.DBtech))
	templates := r.Group("/templates")
	{
		templates.GET("/", templateHandler.GetTemplates)
		templates.POST("/add-template", templateHandler.AddTemplate)
		templates.PUT("/:name/:channel", templateHandler.UpdateTemplateByNameAndChannel)
		templates.GET("/id/:id", templateHandler.GetTemplateByID)
		templates.DELETE("/id/:id", templateHandler.DeleteTemplate)
	}

	log.Printf("Server running on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
