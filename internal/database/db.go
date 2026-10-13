package database

import (
	"fmt"
	"strconv"
	"strings"
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
	DBMarketing *gorm.DB
	// DBZapCashV1 is the Core pool for ZapCash channel Input/Output audit rows.
	// It stays nil when the toggle is off or the connection cannot be opened.
	DBZapCashV1 *gorm.DB
)

const (
	defaultZapCashV1MaxOpenConns = 30
	defaultZapCashV1MaxIdleConns = 15
	defaultMarketingMaxOpenConns = 50
	defaultMarketingMaxIdleConns = 25
)

const (
	Tech      string = "tech"
	Analytics string = "analytics"
	Marketing string = "marketing"
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

func connectMarketingDB(config models.Config) error {
	if strings.TrimSpace(config.DbNameMarketing) == "" {
		utils.Info("Marketing DB name not set, skipping Marketing SQL Server connection.")
		return nil
	}
	if DBMarketing != nil {
		utils.Info("Marketing DB already connected, skipping initialization.")
		return nil
	}

	server := strings.TrimSpace(config.DbServerMarketing)
	if server == "" {
		server = config.DbServerAnalytical
	}
	port := strings.TrimSpace(config.DbPortMarketing)
	if port == "" {
		port = config.DbPortAnalytical
	}
	if port == "" {
		port = "1433"
	}
	user := strings.TrimSpace(config.DbUserMarketing)
	if user == "" {
		user = config.DbUserAnalytical
	}
	password := strings.TrimSpace(config.DbPasswordMarketing)
	if password == "" {
		password = config.DbPasswordAnalytical
	}

	dsn := GetDSN(user, password, server, port, config.DbNameMarketing)
	db, err := gorm.Open(sqlserver.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to Marketing DB: %w", err)
	}
	if err := applyPoolLimits(db, config.DbMaxOpenConnsMarketing, config.DbMaxIdleConnsMarketing, config.DbConnMaxLifetime, "Marketing DB", "DB_MAX_OPEN_CONNS_MARKETING", "DB_MAX_IDLE_CONNS_MARKETING", defaultMarketingMaxOpenConns, defaultMarketingMaxIdleConns); err != nil {
		return err
	}
	DBMarketing = db
	utils.Info("Database connection established for Marketing DB.")
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
	case Marketing:
		return connectMarketingDB(config)
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

func PingMarketingDB() error {
	if DBMarketing == nil {
		return nil
	}
	return pingDatabase(DBMarketing, "Marketing DB")
}

// PingAnalyticsDB pings the Analytics database connection
func PingAnalyticsDB() error {
	return pingDatabase(DBanalytics, "Analytics DB")
}

func applyPoolConfig(db *gorm.DB, config models.Config, dbName string) error {
	return applyPoolLimits(db, config.DbMaxOpenConns, config.DbMaxIdleConns, config.DbConnMaxLifetime, dbName, "DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", 15, 5)
}

func applyPoolLimits(db *gorm.DB, openRaw, idleRaw, lifetimeRaw, dbName, openName, idleName string, defaultOpen, defaultIdle int) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB for %s: %w", dbName, err)
	}

	maxOpen := poolLimit(openRaw, openName, dbName, defaultOpen)
	maxIdle := poolLimit(idleRaw, idleName, dbName, defaultIdle)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)

	maxLifetime := 15 * time.Minute
	lifetimeRaw = strings.TrimSpace(lifetimeRaw)
	if lifetimeRaw == "" {
		utils.Info(fmt.Sprintf("DB_CONN_MAX_LIFETIME_MINUTES not set for %s, using default %s", dbName, maxLifetime))
	} else if parsed, parseErr := strconv.Atoi(lifetimeRaw); parseErr != nil || parsed < 1 {
		utils.Info(fmt.Sprintf("Invalid DB_CONN_MAX_LIFETIME_MINUTES for %s, using default %s", dbName, maxLifetime))
	} else {
		maxLifetime = time.Duration(parsed) * time.Minute
	}
	sqlDB.SetConnMaxLifetime(maxLifetime)
	utils.Debug(fmt.Sprintf("DB pool configured for %s: max_open=%d max_idle=%d max_lifetime=%s", dbName, maxOpen, maxIdle, maxLifetime))
	return nil
}

// ParsePoolLimit converts a pool-size setting. Blank, zero, and invalid values use fallback.
func ParsePoolLimit(raw, name, dbName string, fallback int) int {
	return poolLimit(raw, name, dbName, fallback)
}

func poolLimit(raw, name, dbName string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		utils.Info(fmt.Sprintf("%s not set for %s, using default %d", name, dbName, fallback))
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		utils.Info(fmt.Sprintf("Invalid %s for %s, using default %d", name, dbName, fallback))
		return fallback
	}
	return parsed
}

// ConnectZapCashV1IfEnabled opens the Core audit pool when the ZapCash v1
// toggle is on. An incomplete DSN or a connection failure returns an error
// and leaves DBZapCashV1 nil. The caller logs that error once and continues.
// The toggle off path returns nil and does not connect.
func ConnectZapCashV1IfEnabled(config models.Config) error {
	if !strings.EqualFold(strings.TrimSpace(config.ZapCashV1InputOutputWrite), "true") {
		return nil
	}
	if DBZapCashV1 != nil {
		utils.Info("ZapCash v1 audit DB already connected, skipping initialization.")
		return nil
	}

	host := strings.TrimSpace(config.DbServerZapCashV1)
	user := strings.TrimSpace(config.DbUserZapCashV1)
	password := strings.TrimSpace(config.DbPasswordZapCashV1)
	name := strings.TrimSpace(config.DbNameZapCashV1)
	if name == "" {
		name = "Core"
	}
	if host == "" || user == "" || password == "" {
		return fmt.Errorf("incomplete zapcash v1 DSN: host, user, and password are required")
	}

	db, err := gorm.Open(mysql.Open(GetMySQLDSN(user, password, host, name)), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to ZapCash v1 audit DB: %w", err)
	}
	if err := applyZapCashV1Pool(db, config); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return err
	}
	DBZapCashV1 = db
	utils.Info("Database connection established for ZapCash v1 audit DB.")
	return nil
}

func applyZapCashV1Pool(db *gorm.DB, config models.Config) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB for ZapCash v1 audit DB: %w", err)
	}

	maxOpen := poolLimit(config.DbMaxOpenConnsZapCashV1, "DB_MAX_OPEN_CONNS_ZAPCASH_V1", "ZapCash v1 audit DB", defaultZapCashV1MaxOpenConns)
	maxIdle := poolLimit(config.DbMaxIdleConnsZapCashV1, "DB_MAX_IDLE_CONNS_ZAPCASH_V1", "ZapCash v1 audit DB", defaultZapCashV1MaxIdleConns)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	ConfigureAuditInsertSlots(maxOpen)

	maxLifetime := 15 * time.Minute
	lifetimeRaw := strings.TrimSpace(config.DbConnMaxLifetime)
	if lifetimeRaw == "" {
		utils.Info(fmt.Sprintf("DB_CONN_MAX_LIFETIME_MINUTES not set for ZapCash v1 audit DB, using default %s", maxLifetime))
	} else if parsed, parseErr := strconv.Atoi(lifetimeRaw); parseErr != nil || parsed < 1 {
		utils.Info(fmt.Sprintf("Invalid DB_CONN_MAX_LIFETIME_MINUTES for ZapCash v1 audit DB, using default %s", maxLifetime))
	} else {
		maxLifetime = time.Duration(parsed) * time.Minute
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(maxLifetime)
	utils.Debug(fmt.Sprintf("DB pool configured for ZapCash v1 audit DB: max_open=%d max_idle=%d max_lifetime=%s", maxOpen, maxIdle, maxLifetime))
	return nil
}
