package cache

import (
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func LoadConsumerDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	// Store auth data into cache
	storeDataIntoCache(AuthDetails, config.BasicAuthTableName, database.DBtechRead)

	// Store Vendors Data into cache
	StoreMappedDataIntoCache(VendorsData, config.VendorTable, "Name", "Channel", database.DBtechRead)

	StoreMappedDataIntoCache(ClientsData, config.ClientsTable, "Name", "Channel", database.DBtechRead)

	StoreMappedDataIntoCache(TemplateDetailsData, config.TemplateDetailsTable, "Process", "Stage", database.DBtechRead)

	// storeDataIntoCache(ActiveVendors, config.VendorTable, database.DBtech)

	// Store auth data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtech)

	// Store Vendors Data into cache
	// StoreMappedDataIntoCache(RcsTemplateAppData, config.RcsTemplateAppIdTable, "AppId", "", database.DBtech)

}
