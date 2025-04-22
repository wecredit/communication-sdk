package cache

import (
	"dev.azure.com/wctec/communication-engine/sdk/internal/database"
	"dev.azure.com/wctec/communication-engine/sdk/internal/models"
	"dev.azure.com/wctec/communication-engine/sdk/internal/utils"
)

func LoadApiDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	// Store auth data into cache
	storeDataIntoCache(AuthDetails, config.BasicAuthTableName, database.DBtech)

}