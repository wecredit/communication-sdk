package cache

import (
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func LoadConsumerDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	// Store Vendors Data into cache
	StoreMappedDataIntoCache(VendorsData, config.VendorTable, "Name", "Channel", database.DBtech)
	// storeDataIntoCache(ActiveVendors, config.VendorTable, database.DBtech)

	// Store auth data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtech)

	// Store Vendors Data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtech)

}


