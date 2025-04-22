package utils

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// Log levels
const (
	DEBUG  = iota // 0
	INFO          // 1
	WARN          // 2
	ERROR         // 3
	NOLOGS        // 4
)

var logLevel = INFO

var Logger = log.New(os.Stdout, "LOG: ", log.Ldate|log.Ltime|log.Lshortfile)

func init() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: No .env file found: %v", err)
	}

	// Read log level from environment variable
	level := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(level) {
	case variables.Debug:
		logLevel = DEBUG
	case variables.Info:
		logLevel = INFO
	case variables.Warn:
		logLevel = WARN
	case variables.Error:
		logLevel = ERROR
	case variables.NoLogs:
		logLevel = NOLOGS
	default:
		logLevel = NOLOGS // Default to NOLOGS if no valid level is set
	}
}

// Error logs an error message if the level is set to ERROR or lower
func Error(err error) {
	if logLevel <= ERROR {
		Logger.Println("ERROR: " + err.Error())
	}
}

// Warning logs a warning message if the level is set to WARN or lower
func Warn(message string) {
	if logLevel <= WARN {
		Logger.Println("WARN: " + message)
	}
}

// Info logs an informational message if the level is set to INFO or lower
func Info(message string) {
	if logLevel <= INFO {
		Logger.Println("INFO: " + message)
	}
}

// Debug logs a debug message if the level is set to DEBUG
func Debug(message string) {
	if logLevel <= DEBUG {
		Logger.Println("DEBUG: " + message)
	}
}
