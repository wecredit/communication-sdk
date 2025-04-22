package cache

import (
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func LoadApiDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	// Store auth data into cache
	storeDataIntoCache(AuthDetails, config.BasicAuthTableName, database.DBtech)

}
