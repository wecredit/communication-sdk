package cache

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/dgraph-io/ristretto"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
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

// Global cache
var ChannelVendorSlots = map[string]map[string][100]string{} // for fast weighted lookup
// var channelActiveVendors = map[string][]Vendor{}  // optional if needed elsewhere

type Vendor struct {
	Name    string
	Channel string
	Client  string
	Status  int
	Weight  int64
}

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
		log.Println("Cache not initialized. Please call InitializeCache first.")
		return nil
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

func StoreMappedDataIntoCache(key, tableName, columnNameToBeUsedAsKey, suffixColumnName string, db *gorm.DB) {
	data, err := database.GetDataFromTable(tableName, db)
	if err != nil {
		utils.Error(fmt.Errorf("failed to fetch initial data for cache: %v", err))
		return
	}

	utils.Info(fmt.Sprintf("fetched %d entries from DB", len(data)))

	mappedData := make(map[string]map[string]interface{})
	idIndex := make(map[uint]string) // Map Id to cache key

	for _, row := range data {
		keyVal, ok := row[columnNameToBeUsedAsKey]
		if !ok {
			utils.Warn(fmt.Sprintf("skipped a row: column '%s' missing", columnNameToBeUsedAsKey))
			continue
		}

		keyStr := fmt.Sprintf("%s:%v", columnNameToBeUsedAsKey, keyVal)
		if suffixColumnName != "" {
			if suffixVal, ok := row[suffixColumnName]; ok && tableName == config.Configs.TemplateDetailsTable {
				// stageFloat, _ := strconv.ParseFloat(string(suffixVal.([]uint8)), 64)
				// stageFloat, _ := strconv.ParseFloat(suffixVal.(string), 64)

				// Stage is now already parsed as float64 from database function
				stageFloat := suffixVal.(float64)
				keyStr = fmt.Sprintf("%s|%s:%.2f", keyStr, suffixColumnName, stageFloat)
			} else if suffixVal, ok := row[suffixColumnName]; ok {
				keyStr = fmt.Sprintf("%s|%s:%v", keyStr, suffixColumnName, suffixVal)
			} else {
				utils.Warn(fmt.Sprintf("suffix column '%s' missing for a row", suffixColumnName))
			}
		}

		if tableName == config.Configs.TemplateDetailsTable {
			keyStr = fmt.Sprintf("%s|Client:%v|Channel:%v|Vendor:%v", keyStr, row["Client"], row["Channel"], row["Vendor"])
		}

		if tableName == config.Configs.VendorTable {
			clientStr := strings.ToLower(strings.TrimSpace(row["Client"].(string)))
			keyStr = fmt.Sprintf("%s|Client:%s", keyStr, clientStr)
		}

		mappedData[keyStr] = row

		// Build Id index
		if id, ok := row["Id"].(int64); ok {
			idIndex[uint(id)] = keyStr
		}
	}

	if !GetCache().Set(key, mappedData) {
		utils.Error(fmt.Errorf("failed to set data in cache for key: %v", key))
		return
	}

	// Store Id index
	idIndexKey := key + ":IdIndex"
	if !GetCache().Set(idIndexKey, idIndex) {
		utils.Error(fmt.Errorf("failed to set Id index in cache for key: %v", idIndexKey))
		return
	}

	if key == VendorsData {
		TransformAndCacheVendors(mappedData)
	}
	utils.Info(fmt.Sprintf("Cache initialized successfully for key: %s", key))
}

func TransformAndCacheVendors(raw map[string]map[string]interface{}) {
	temp := make(map[string]map[string][]Vendor)
	// Step 1: Group by channel & client with only active vendors
	for _, row := range raw {
		status := (row["Status"].(int64))
		if status != variables.Active {
			continue
		}

		name := strings.ToUpper(strings.TrimSpace(row["Name"].(string)))
		channel := strings.ToUpper(strings.TrimSpace(row["Channel"].(string)))
		client := strings.ToLower(strings.TrimSpace(row["Client"].(string)))
		weight := row["Weight"].(int64)
		if weight <= 0 {
			continue
		}

		if _, ok := temp[channel]; !ok {
			temp[channel] = make(map[string][]Vendor)
		}

		v := Vendor{
			Name:    name,
			Channel: channel,
			Client:  client,
			Status:  1,
			Weight:  weight,
		}
		temp[channel][client] = append(temp[channel][client], v)
	}

	// Step 2: Pre-compute vendor slots for each channel & client
	final := make(map[string]map[string][100]string)
	for channel, clientVendors := range temp {
		if _, ok := final[channel]; !ok {
			final[channel] = make(map[string][100]string)
		}
		for client, vendors := range clientVendors {
			var slots [100]string
			var pos int64 = 0
			for _, v := range vendors {
				end := pos + v.Weight
				if end > 100 {
					end = 100
				}
				for i := pos; i < end; i++ {
					slots[i] = v.Name
				}
				pos = end
				if pos >= 100 {
					break
				}
			}
			final[channel][client] = slots
		}
	}

	// Step 3: Store in global vars
	ChannelVendorSlots = final
}

// Get fetches the data from the cache for a given key
func (c *Cache) GetMappedData(key string) (map[string]map[string]interface{}, bool) {
	value, found := c.store.Get(key)
	if !found {
		fmt.Println("Cache key not found:", key)
		return nil, false
	}

	// Convert slice to map using lender names as keys
	mappedData := value.(map[string]map[string]interface{})

	return mappedData, true
}

func (c *Cache) GetMappedIdData(key string) (map[uint]string, bool) {
	value, found := c.store.Get(key)
	if !found {
		fmt.Println("Cache key not found:", key)
		return nil, false
	}

	// Convert slice to map using lender names as keys
	mappedData := value.(map[uint]string)

	return mappedData, true
}
