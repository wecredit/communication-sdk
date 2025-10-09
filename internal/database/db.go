package database

import (
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

var (
	DBanalytics *gorm.DB
	DBtechRead  *gorm.DB
	DBtechWrite *gorm.DB
)

const (
	Tech      string = "tech"
	Analytics string = "analytics"
)

// GetDSN generates the DSN string for the database connection
func GetDSN(user, password, server, port, database string) string {
	return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, password, server, port, database)
}

func GetMySQLDSN(username, password, server, database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		username, password, server, database)
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
		// Connect to Analytical DB
		DBanalytics, err = gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to Analytical DB: %w", err)
		}
		utils.Info("Database connection established for Analytical DB.")

	case Tech:
		dsnRead := GetMySQLDSN(
			config.DbUserTech,
			config.DbPasswordTech,
			config.DbServerTechRead,
			config.DbNameTech,
		)

		fmt.Println("DSN Read: ", dsnRead)

		// Connect to Tech DB
		DBtechRead, err = gorm.Open(mysql.Open(dsnRead), &gorm.Config{})
		if err != nil {
			utils.Error(err)
			return fmt.Errorf("failed to connect to Tech Read DB: %w", err)
		}

		// fmt.Println("DBtechRead: ", DBtechRead)

		utils.Info("Database connection established for Tech Read DB.")

		dsnWrite := GetMySQLDSN(
			config.DbUserTech,
			config.DbPasswordTech,
			config.DbServerTechWrite,
			config.DbNameTech,
		)
		// Connect to Tech DB
		DBtechWrite, err = gorm.Open(mysql.Open(dsnWrite), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to Tech Write DB: %w", err)
		}
		utils.Info("Database connection established for Tech Write DB.")

	default:
		return fmt.Errorf("invalid database type: %s", dbType)
	}

	return nil
}

func PingTechReadDB() error {
	if DBtechRead == nil {
		return errors.New("tech Read DB is not initialized")
	}
	sqlDB, err := DBtechRead.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func PingTechWriteDB() error {
	if DBtechWrite == nil {
		return errors.New("tech Write DB is not initialized")
	}
	sqlDB, err := DBtechWrite.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
