package cache

import (
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func LoadApiDataIntoCache(config models.Config) {
	// Initializing Cache Items
	utils.Info("Initializing Ristretto cache...")

	// Initialize the global cache
	InitializeCache()

	// Store auth data into cache
	storeDataIntoCache(AuthDetails, config.BasicAuthTableName, database.DBtech)

}
