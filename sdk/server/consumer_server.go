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

	vendorService := services.NewVendorService(database.DBtech)
	vendorHandler := handlers.NewVendorHandler(vendorService)

	v := r.Group("/vendors")
	{
		v.GET("/", vendorHandler.GetVendors)       // Optional filter: ?channel=WHATSAPP
		v.GET("/:id", vendorHandler.GetVendorByID) // Get by ID
		v.POST("/add", vendorHandler.AddVendor)
		v.PUT("/:name/:channel", vendorHandler.UpdateVendorByNameAndChannel)
		v.DELETE("/:id", vendorHandler.DeleteVendor)
	}

	log.Printf("Server running on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
