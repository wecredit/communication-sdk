package server

import (
	"fmt"
	"log"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/cron"
	"github.com/wecredit/communication-sdk/health"
	push "github.com/wecredit/communication-sdk/internal/channels/push"
	pushAudit "github.com/wecredit/communication-sdk/internal/channels/push/audit"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/handlers"
	"github.com/wecredit/communication-sdk/internal/middleware"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/internal/services/monitoring"
	"github.com/wecredit/communication-sdk/sdk/utils"
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
	monitoring.Init()
	if err := pushAudit.Init(
		database.DBtechWrite,
		config.Configs.PushInputAuditTable,
		config.Configs.PushOutputTable,
		nil,
	); err != nil {
		utils.Error(fmt.Errorf("failed to initialize PUSH audit dispatcher: %w", err))
	}
	if err := push.Init(config.Configs, database.DBtechWrite); err != nil {
		utils.Error(fmt.Errorf("failed to initialize PUSH service: %w", err))
	}

	go services.ConsumerService(config.Configs.AwsQueueUrl)
	go cron.StartMidnightResetCron()
	utils.Debug(fmt.Sprintf("Starting Consumer Server on port %s", port))

	// Set up Gin router
	r := gin.Default()

	r.GET("/health", health.HealthCheckHandler(port))

	adminBasicAuth, err := middleware.NewAdminBasicAuth(config.Configs.CommAdminUsername, config.Configs.CommAdminPassword)
	if err != nil {
		log.Fatalf("Failed to configure communication admin authentication: %v", err)
	}
	commScopeConfig := middleware.NewCommScopeConfig(config.Configs.CommSuperAdminRoles, config.Configs.CommClientRolePrefix)
	commAdminScope, err := middleware.NewCommAdminScopeMiddleware(commScopeConfig, config.Configs.CommIdentitySecret)
	if err != nil {
		log.Fatalf("Failed to configure communication admin scope: %v", err)
	}
	admin := r.Group("")
	admin.Use(adminBasicAuth)
	admin.Use(commAdminScope)

	vendorHandler := handlers.NewVendorHandler(apiServices.NewVendorService(database.DBtechRead)) // Create handler for vendors passing them database object
	vendors := r.Group("/vendors")
	{
		vendors.GET("/", vendorHandler.GetVendors) // endpoint:- /vendors; filter: ?channel=WHATSAPP
		vendors.POST("/add-vendor", vendorHandler.AddVendor)
		vendors.PUT("/:name/:channel", vendorHandler.UpdateVendorByNameAndChannel)
		vendors.GET("/id/:id", vendorHandler.GetVendorByID) // endpoint:- /vendors/{id};
		vendors.DELETE("/id/:id", vendorHandler.DeleteVendor)
	}

	clientHandler := handlers.NewClientHandler(apiServices.NewClientService(database.DBtechRead)) // Create handler for vendors passing them database object
	clients := r.Group("/clients")
	{
		clients.GET("/", clientHandler.GetClients)
		clients.POST("/add-client", clientHandler.AddClient)
		clients.PUT("/:name/:channel", clientHandler.UpdateClientByNameAndChannel)
		clients.GET("/id/:id", clientHandler.GetClientByID)
		clients.DELETE("/id/:id", clientHandler.DeleteClient)
		clients.POST("/validate-client", clientHandler.ValidateClient)
	}

	templateHandler := handlers.NewTemplateHandler(apiServices.NewTemplateService(database.DBtechRead, database.DBtechWrite))
	templates := admin.Group("/templates")
	{
		templates.GET("/", templateHandler.GetTemplates)
		templates.POST("/add-template", templateHandler.AddTemplate)
		templates.PUT("/id/:id", templateHandler.UpdateTemplateById)
		templates.GET("/id/:id", templateHandler.GetTemplateByID)
		templates.DELETE("/id/:id", templateHandler.DeleteTemplate)
	}

	stageConfigurationHandler := handlers.NewStageConfigurationHandler(apiServices.NewStageConfigurationService(database.DBtechRead, database.DBtechWrite))
	stageConfigurations := admin.Group("/stage-configurations")
	{
		stageConfigurations.POST("", stageConfigurationHandler.Create)
		stageConfigurations.PUT("/lender-schedule/id/:id", stageConfigurationHandler.Update)
	}
	lenderSchedules := admin.Group("/lender-schedules")
	{
		lenderSchedules.GET("", stageConfigurationHandler.ListLenderSchedules)
		lenderSchedules.GET("/id/:id", stageConfigurationHandler.GetLenderSchedule)
		lenderSchedules.DELETE("/id/:id", stageConfigurationHandler.DeleteLenderSchedule)
	}
	stageMappings := admin.Group("/stage-mappings")
	{
		stageMappings.GET("", stageConfigurationHandler.ListStageMappings)
		stageMappings.DELETE("/id/:id", stageConfigurationHandler.DeleteStageMapping)
	}

	// if err := r.Run(":" + port); err != nil {
	if err := r.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
