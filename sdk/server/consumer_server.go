package server

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/sdk/config"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
)

func StartConsumer(port string) {
	go services.ConsumerService(10, config.Configs.QueueTopicName, config.Configs.QueueSubscriptionName)

	// Retrieve sub-lender details from the cache
	vendorDetails, ok := cache.GetCache().GetMappedData(cache.VendorsData)
	if !ok {
		fmt.Println("Vendor data not found or failed to cast cache data")
	}

	fmt.Println("Data Cache:", vendorDetails)

	// Set up Gin router
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "consumer API is running",
		})
	})

	// r.POST("/api/auth", handleAuth)

	log.Printf("Server running on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
