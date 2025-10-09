package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	env "github.com/wecredit/communication-sdk/sdk/constant"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// Log levels
const (
	DEBUG  = iota
	INFO
	WARN
	ERROR
	NOLOGS
)

var logLevel = INFO
var Logger *log.Logger

func init() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: No .env file found: %v", err)
	}

	// Create logs directory if not exists
	logDir := "logs"
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	// Open log file
	logFilePath := filepath.Join(logDir, "app.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	// MultiWriter to log both to console and file
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	Logger = log.New(multiWriter, "LOG: ", log.Ldate|log.Ltime|log.Lshortfile)

	// Set log level
	level := env.LOG_LEVEL
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
		logLevel = INFO // Default to INFO if not set
	}

	Logger.Printf("Logger initialized with level: %s", strings.ToUpper(level))
}

// Error logs an error message
func Error(err error) {
	if logLevel <= ERROR {
		Logger.Println("ERROR:", err)
	}
}

// Warn logs a warning message
func Warn(message string) {
	if logLevel <= WARN {
		Logger.Println("WARN:", message)
	}
}

// Info logs an informational message
func Info(message string) {
	if logLevel <= INFO {
		Logger.Println("INFO:", message)
	}
}

// Debug logs a debug message
func Debug(message string) {
	if logLevel <= DEBUG {
		Logger.Println("DEBUG:", message)
	}
}
