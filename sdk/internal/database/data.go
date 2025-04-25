package database

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
	"gorm.io/gorm"
)

// GetBasicAuthData fetches data from the BasicAuth table and returns it
func GetDataFromTable(tableName string, db *gorm.DB) ([]map[string]interface{}, error) {
	if tableName == "" {
		return nil, fmt.Errorf("table name cannot be empty")
	}

	var results []map[string]interface{}

	// Execute raw SQL to fetch all data from the table
	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from table %s: %w", tableName, err)
	}
	defer rows.Close()

	// Parse rows into a slice of maps
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	for rows.Next() {
		// Create a map to store column data
		row := make(map[string]interface{})
		columnPointers := make([]interface{}, len(columns))

		for i := range columns {
			var colData interface{}
			columnPointers[i] = &colData
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		for i, colName := range columns {
			row[colName] = *(columnPointers[i].(*interface{}))
		}

		results = append(results, row)
	}

	if len(results) < 1 {
		utils.Info("No data found")
	}

	// Log the result for debugging
	// jsonData, _ := json.Marshal(results) // Optional: Serialize for readability
	log.Printf("Fetched data from table '%s'", tableName)

	return results, nil
}

// GetWhatsappProcessData fetches records based on the provided process name
func GetWhatsappProcessData(db *gorm.DB, process, source string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	var query *gorm.DB

	switch source {
	case variables.TIMES:
		query = db.Table("API-HITS.dbo.whatsapp_process_temp").
			Where("Process LIKE ?", process).
			Where("api_source = ?", "times").
			Where("IsActive = ?", true).
			Where("CAST(Execution_date AS DATE) = CAST(GETDATE() AS DATE)")

	case variables.SINCH:
		query = db.Table("API-HITS.dbo.whatsapp_process_temp").
			Where("Process LIKE ?", process).
			Where("api_source = ?", "sinch").
			Where("IsActive = ?", true).
			Where("CAST(Execution_date AS DATE) = CAST(GETDATE() AS DATE)")
	}
	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}
