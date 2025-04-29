package cache

import (
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/ristretto"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

// Cache structure with Ristretto cache store
type Cache struct {
	store *ristretto.Cache
}

var (
	appCache *Cache    // Global cache variable
	once     sync.Once // Ensure initialization happens only once
)

// InitializeCache initializes the global cache instance
func InitializeCache() {
	once.Do(func() { // Singleton pattern to ensure only one instance is created
		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: 1e4,     // Number of keys to track for eviction
			MaxCost:     1 << 20, // 1 MB cache size
			BufferItems: 64,      // Number of keys per Get buffer
		})
		if err != nil {
			log.Fatalf("Failed to create cache: %v", err)
		}
		appCache = &Cache{store: cache}
		// log.Println("Cache initialized successfully.")
	})
}

// GetCache returns the global cache instance
func GetCache() *Cache {
	if appCache == nil {
		log.Fatal("Cache not initialized. Please call InitializeCache first.")
	}
	return appCache
}

// Set stores a list of maps in the cache with a specific key
func (c *Cache) Set(key string, data interface{}) bool {
	// Use a fixed cost; adjust based on your application's needs
	return c.store.Set(key, data, 1)
}

// Get fetches the data from the cache for a given key
func (c *Cache) Get(key string) ([]map[string]interface{}, bool) {
	value, found := c.store.Get(key)
	if !found {
		return nil, false
	}
	// Type assertion to []map[string]interface{}
	if result, ok := value.([]map[string]interface{}); ok {
		return result, true
	}
	return nil, false
}

func storeDataIntoCache(key, tableName string, db *gorm.DB) {
	data, err := database.GetDataFromTable(tableName, db)
	if err != nil {
		utils.Error(fmt.Errorf("failed to fetch initial data for cache: %v", err))
	}
	// Store the data in the cache
	if !GetCache().Set(key, data) {
		utils.Error(fmt.Errorf("failed to set initial data in cache for: %v", key))
	}

	utils.Info(fmt.Sprint("Cache initialized successfully for: ", key))
}

func StoreMappedDataIntoCache(key, tableName, columnNameToBeUsedAsKey string, db *gorm.DB) {
	// Step 1: Fetch all data from DB
	data, err := database.GetDataFromTable(
		tableName,
		db,
	)
	if err != nil {
		utils.Error(fmt.Errorf("failed to fetch initial data for cache: %v", err))
		return
	}

	// Optional: Log size of data
	utils.Info(fmt.Sprintf("fetched %d entries from DB", len(data)))

	// Step 2: Transform into map using a specific column as key
	mappedData := make(map[string]map[string]interface{})

	for _, row := range data {
		if keyVal, ok := row[columnNameToBeUsedAsKey]; ok {
			keyStr := fmt.Sprintf("%v", keyVal) // safely convert to string even if int or other type
			mappedData[keyStr] = row
		} else {
			utils.Warn(fmt.Sprintf("skipped a row: column '%s' missing", columnNameToBeUsedAsKey))
		}
	}

	// Step 3: Store into cache
	if !GetCache().Set(key, mappedData) {
		utils.Error(fmt.Errorf("failed to set data in cache for key: %v", key))
		return
	}

	utils.Info(fmt.Sprintf("Cache initialized successfully for key: %s", key))
}
