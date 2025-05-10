package services

import (
	"hash/fnv"

	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
)

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

func GetVendorByChannel(channel, idempotencyKey string) string {
	h := fnv.New32a()
	h.Write([]byte(idempotencyKey))
	val := int(h.Sum32() % 100)

	if slots, ok := cache.ChannelVendorSlots[channel]; ok {
		return slots[val]
	}
	return "UNKNOWN"
}
