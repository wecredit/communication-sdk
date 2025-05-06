package database

import (
	"fmt"
	"log"
	"strings"

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

func GetRcsAppId(db *gorm.DB, AppId string) (map[string]interface{}, error) {
	var result map[string]interface{}
	var query *gorm.DB
	query = db.Table("RcsTemplateAppId").
		Where("AppId LIKE ?", AppId)

	if err := query.Find(&result).Error; err != nil {
		return nil, err
	}

	return result, nil
}

func GetTemplateDetails(db *gorm.DB, process, channel, vendor string, stage int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	var query *gorm.DB

	switch vendor {
	case variables.TIMES:
		query = db.Table("TemplateDetails").
			Where("Process LIKE ?", process).
			Where("Stage = ?", stage).
			Where("Channel = ?", channel).
			Where("Vendor = ?", "TIMES").
			Where("IsActive = ?", true)

	case variables.SINCH:
		query = db.Table("TemplateDetails").
			Where("Process LIKE ?", process).
			Where("Stage = ?", stage).
			Where("Channel = ?", channel).
			Where("Vendor = ?", "SINCH").
			Where("IsActive = ?", true)
	}

	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}

	fmt.Println("Results", results)

	return results, nil
}

// GetWhatsappProcessData fetches records based on the provided process name
func GetWhatsappProcessData(db *gorm.DB, process, vendor string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	var query *gorm.DB

	fmt.Println("db:", db)
	fmt.Println("Process:", process, vendor)

	switch vendor {
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

	fmt.Println("Query:", query)

	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}
	fmt.Println("Results:", results)

	return results, nil
}

// InsertData inserts data into the given table name using a transaction
func InsertData(tableName string, db *gorm.DB, data map[string]interface{}) error {
	session := db.Session(&gorm.Session{NewDB: true})

	if tableName == "" {
		return fmt.Errorf("table name cannot be empty")
	}

	if len(data) == 0 {
		return fmt.Errorf("data cannot be empty")
	}

	// Start the transaction manually
	tx := session.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	// Ensure rollback on panic
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r) // Re-throw panic after rollback
		}
	}()

	// Construct the columns and values part of the SQL query
	var columns []string
	var placeholders []string
	var values []interface{}

	for col, value := range data {
		columns = append(columns, col)
		placeholders = append(placeholders, "?") // Placeholder for SQL query
		values = append(values, value)
	}

	// Add ROWLOCK hint to enforce row-level locking
	query := fmt.Sprintf(
		"INSERT INTO %s WITH (ROWLOCK) (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Execute the query with the values
	result := tx.Exec(query, values...)
	if result.Error != nil {
		tx.Rollback() // Explicit rollback on error
		return fmt.Errorf("failed to insert data into table %s: %w", tableName, result.Error)
	}

	// Explicitly commit the transaction
	if err := tx.Commit().Error; err != nil {
		tx.Rollback() // Rollback if commit fails
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Log success
	utils.Info(fmt.Sprintf("Successfully inserted data into table '%s'", tableName))
	return nil
}
