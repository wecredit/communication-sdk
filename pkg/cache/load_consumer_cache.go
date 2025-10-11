package cache

import (
	"fmt"

	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func LoadConsumerDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	if err := storeDataIntoCache(AuthDetails, config.BasicAuthTableName, database.DBtechRead); err != nil {
		utils.Error(fmt.Errorf("cache initialization failed for auth details: %v", err))
		// optionally trigger a background retry or health check flag
	}
	
	// Store Vendors Data into cache
	StoreMappedDataIntoCache(VendorsData, config.VendorTable, "Name", "Channel", database.DBtechRead)

	StoreMappedDataIntoCache(ClientsData, config.ClientsTable, "Name", "Channel", database.DBtechRead)

	StoreMappedDataIntoCache(TemplateDetailsData, config.TemplateDetailsTable, "Process", "Stage", database.DBtechRead)

	// storeDataIntoCache(ActiveVendors, config.VendorTable, database.DBtechRead)

	// Store auth data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtechRead)

	// Store Vendors Data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtech)

}
