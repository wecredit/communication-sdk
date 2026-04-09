package database

import (
	"fmt"
	"strconv"
	"time"

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

// Database connection types
const (
	ConnectionTypeRead  = "read"
	ConnectionTypeWrite = "write"
)

// GetDSN generates the DSN string for the database connection
func GetDSN(user, password, server, port, database string) string {
	return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, password, server, port, database)
}

func GetMySQLDSN(username, password, server, database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		username, password, server, database)
}

// connectAnalyticsDB establishes connection to Analytics database
func connectAnalyticsDB(config models.Config) error {
	if DBanalytics != nil {
		utils.Info("Analytical DB already connected, skipping initialization.")
		return nil
	}

	dsn := GetDSN(
		config.DbUserAnalytical,
		config.DbPasswordAnalytical,
		config.DbServerAnalytical,
		config.DbPortAnalytical,
		config.DbNameAnalytical,
	)

	var err error
	DBanalytics, err = gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to Analytical DB: %w", err)
	}

	if err := applyPoolConfig(DBanalytics, config, "Analytical DB"); err != nil {
		return err
	}

	utils.Info("Database connection established for Analytical DB.")
	return nil
}

// connectTechDB establishes connection to Tech database (read or write)
func connectTechDB(connectionType string, config models.Config) error {
	var (
		varDB  **gorm.DB
		server string
		dbName string
	)

	// Determine which database variable and server to use
	switch connectionType {
	case ConnectionTypeRead:
		if DBtechRead != nil {
			utils.Info("Tech Read DB already connected, skipping initialization.")
			return nil
		}
		varDB = &DBtechRead
		server = config.DbServerTechRead
		dbName = "Tech Read DB"
	case ConnectionTypeWrite:
		if DBtechWrite != nil {
			utils.Info("Tech Write DB already connected, skipping initialization.")
			return nil
		}
		varDB = &DBtechWrite
		server = config.DbServerTechWrite
		dbName = "Tech Write DB"
	default:
		return fmt.Errorf("invalid connection type: %s", connectionType)
	}

	dsn := GetMySQLDSN(
		config.DbUserTech,
		config.DbPasswordTech,
		server,
		config.DbNameTech,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", dbName, err)
	}

	if err := applyPoolConfig(db, config, dbName); err != nil {
		return err
	}

	*varDB = db
	utils.Info(fmt.Sprintf("Database connection established for %s.", dbName))
	return nil
}

// ConnectDB initializes the database connection pool for the given database type
func ConnectDB(dbType string, config models.Config) error {
	switch dbType {
	case Analytics:
		return connectAnalyticsDB(config)
	case Tech:
		// Connect both read and write connections for Tech DB
		if err := connectTechDB(ConnectionTypeRead, config); err != nil {
			return err
		}
		if err := connectTechDB(ConnectionTypeWrite, config); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid database type: %s", dbType)
	}
}

// pingDatabase is a generic function to ping any database connection
func pingDatabase(db *gorm.DB, dbName string) error {
	if db == nil {
		return fmt.Errorf("%s is not initialized", dbName)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB for %s: %w", dbName, err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("ping failed for %s: %w", dbName, err)
	}

	return nil
}

// PingTechReadDB pings the Tech Read database connection
func PingTechReadDB() error {
	return pingDatabase(DBtechRead, "Tech Read DB")
}

// PingTechWriteDB pings the Tech Write database connection
func PingTechWriteDB() error {
	return pingDatabase(DBtechWrite, "Tech Write DB")
}

// PingAnalyticsDB pings the Analytics database connection
func PingAnalyticsDB() error {
	return pingDatabase(DBanalytics, "Analytics DB")
}

func applyPoolConfig(db *gorm.DB, config models.Config, dbName string) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB for %s: %w", dbName, err)
	}

	maxOpen := 15
	if config.DbMaxOpenConns != "" {
		parsed, err := strconv.Atoi(config.DbMaxOpenConns)
		if err != nil {
			utils.Info(fmt.Sprintf("Invalid DB_MAX_OPEN_CONNS for %s, using default %d", dbName, maxOpen))
		} else {
			maxOpen = parsed
		}
	} else {
		utils.Info(fmt.Sprintf("DB_MAX_OPEN_CONNS not set for %s, using default %d", dbName, maxOpen))
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	maxIdle := 5
	if config.DbMaxIdleConns != "" {
		parsed, err := strconv.Atoi(config.DbMaxIdleConns)
		if err != nil {
			utils.Info(fmt.Sprintf("Invalid DB_MAX_IDLE_CONNS for %s, using default %d", dbName, maxIdle))
		} else {
			maxIdle = parsed
		}
	} else {
		utils.Info(fmt.Sprintf("DB_MAX_IDLE_CONNS not set for %s, using default %d", dbName, maxIdle))
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	maxLifetime := 15 * time.Minute
	if config.DbConnMaxLifetime != "" {
		parsed, err := strconv.Atoi(config.DbConnMaxLifetime)
		if err != nil {
			utils.Info(fmt.Sprintf("Invalid DB_CONN_MAX_LIFETIME_MINUTES for %s, using default %s", dbName, maxLifetime))
		} else {
			maxLifetime = time.Duration(parsed) * time.Minute
		}
	} else {
		utils.Info(fmt.Sprintf("DB_CONN_MAX_LIFETIME_MINUTES not set for %s, using default %s", dbName, maxLifetime))
	}
	sqlDB.SetConnMaxLifetime(maxLifetime)
	utils.Debug(fmt.Sprintf("DB pool configured for %s: max_open=%d max_idle=%d max_lifetime=%s", dbName, maxOpen, maxIdle, maxLifetime))

	return nil
}
