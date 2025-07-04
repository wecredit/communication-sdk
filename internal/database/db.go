package database

import (
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

var (
	DBanalytics *gorm.DB
	DBtech      *gorm.DB
)

const (
	Tech      string = "tech"
	Analytics string = "analytics"
)

// GetDSN generates the DSN string for the database connection
func GetDSN(user, password, server, port, database string) string {
	return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, password, server, port, database)
}

// ConnectDB initializes the database connection pool for the given database type
func ConnectDB(dbType string, config models.Config) error {
	var (
		dsn string
		err error
	)

	// Determine configuration based on the database type
	switch dbType {
	case Analytics:
		dsn = GetDSN(
			config.DbUserAnalytical,
			config.DbPasswordAnalytical,
			config.DbServerAnalytical,
			config.DbPortAnalytical,
			config.DbNameAnalytical,
		)
		fmt.Println("Get DSN", dsn)
		// Connect to Analytical DB
		DBanalytics, err = gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to Analytical DB: %w", err)
		}
		fmt.Println("DBAnal:", DBanalytics)
		utils.Info("Database connection established for Analytical DB.")

	case Tech:
		dsn = GetDSN(
			config.DbUserTech,
			config.DbPasswordTech,
			config.DbServerTech,
			config.DbPortTech,
			config.DbNameTech,
		)
		// Connect to Tech DB
		DBtech, err = gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to Tech DB: %w", err)
		}
		utils.Info("Database connection established for Tech DB.")

	default:
		return fmt.Errorf("invalid database type: %s", dbType)
	}

	return nil
}

func PingTechDB() error {
	if DBtech == nil {
		return errors.New("tech DB is not initialized")
	}
	sqlDB, err := DBtech.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
