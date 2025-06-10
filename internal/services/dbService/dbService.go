package services

import (
	"fmt"
	"reflect"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func InsertSdkInputData(sdkModels.CommApiRequestBody) error {

	return nil
}

func MapIntoDbModel(data any) (map[string]interface{}, error) {
	// Create an empty map to hold the results
	result := make(map[string]interface{})

	// Get the value and type of the input
	val := reflect.ValueOf(data)
	typ := reflect.TypeOf(data)

	// Ensure the input is a pointer to a struct
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}

	// Check if the input is actually a struct
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct, got %v", val.Kind())
	}

	// Loop through the struct fields
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Get the GORM tag value (if present)
		gormTag := fieldType.Tag.Get("gorm")
		if gormTag == "" {
			continue // Skip if no GORM tag is present
		}

		// Use the GORM tag as the map key, convert boolean values to 1/0
		if field.Kind() == reflect.Bool {
			if field.Bool() {
				result[gormTag] = 1 // true becomes 1
			} else {
				result[gormTag] = 0 // false becomes 0
			}
		} else {
			// For other types, use the actual field value
			result[gormTag] = field.Interface()
		}
	}

	return result, nil
}
