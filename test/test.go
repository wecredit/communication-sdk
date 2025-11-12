package main

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/pkg/cache"
	sdkServices "github.com/wecredit/communication-sdk/sdk/services"
)

// getVendor returns the vendor name based on hash of idempotency key
func GetVendor(idempotencyKey string) string {
	h := fnv.New32a()
	h.Write([]byte(idempotencyKey))
	hashValue := h.Sum32() % 100

	if hashValue < 99 {
		return "SINCH"
	}
	return "TIMES"
}

type Vendor struct {
	Name   string
	Weight int
}

func GetVendorByWeight(idempotencyKey string, vendors []Vendor) string {
	h := fnv.New32a()
	h.Write([]byte(idempotencyKey))
	hashValue := h.Sum32() % 100

	rangeStart := 0
	for _, v := range vendors {
		rangeEnd := rangeStart + v.Weight
		if int(hashValue) >= rangeStart && int(hashValue) < rangeEnd {
			return v.Name
		}
		rangeStart = rangeEnd
	}
	return "UNKNOWN"
}

func GetVendorByClientAndChannel(channel, client, idempotencyKey string) string {
	h := fnv.New32a()
	h.Write([]byte(idempotencyKey))
	val := int(h.Sum32() % 100)

	channelKey := strings.ToUpper(channel)
	clientKey := strings.ToLower(client)

	if channelSlots, ok := cache.ChannelVendorSlots[channelKey]; ok {
		if slots, ok := channelSlots[clientKey]; ok {
			if vendor := slots[val]; vendor != "" {
				return vendor
			}
		}
		if slots, ok := channelSlots[""]; ok {
			if vendor := slots[val]; vendor != "" {
				return vendor
			}
		}
	}
	return "UNKNOWN"
}

func main() {
	config.LoadConfigs()
	cache.LoadConsumerDataIntoCache(config.Configs)

	// Example idempotency keys
	testKeys := []string{}

	// Generate 1000 test keys (you can change this number as needed)
	for i := 1; i <= 100; i++ {
		testKeys = append(testKeys, sdkServices.GenerateCommID())
	}

	sinchCount := 0
	timesCount := 0
	abcCount := 0

	// Process each key
	for _, key := range testKeys {
		time.Sleep(1 * time.Second)
		vendor := GetVendorByClientAndChannel("WHATSAPP", "creditsea", key)
		if vendor == "SINCH" {
			sinchCount++
		} else if vendor == "TIMES" {
			timesCount++
		}
		// fmt.Printf("Idempotency Key: %s -> Vendor: %s\n", key, vendor)
	}

	total := len(testKeys)
	sinchPercent := float64(sinchCount) * 100 / float64(total)
	timesPercent := float64(timesCount) * 100 / float64(total)
	abcPercent := float64(abcCount) * 100 / float64(total)

	fmt.Println("\n===== Summary =====")
	fmt.Printf("Total Messages: %d\n", total)
	fmt.Printf("SINCH: %d (%.2f%%)\n", sinchCount, sinchPercent)
	fmt.Printf("TIMES: %d (%.2f%%)\n", timesCount, timesPercent)
	fmt.Printf("ABC: %d (%.2f%%)\n", abcCount, abcPercent)
}
