package utils

import (
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/joho/godotenv"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// Log levels
const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	NOLOGS
)

var logLevel atomic.Int32
var Logger = log.New(os.Stdout, "LOG: ", log.Ldate|log.Ltime|log.Lshortfile)

func init() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: No .env file found: %v", err)
	}

	/* 	// Create logs directory if not exists
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
	*/

	level := variables.Info
	SetLogLevel(level)
	Logger.Printf("Logger initialized with level: %s", strings.ToUpper(level))
}

// SetLogLevel applies a runtime log level, including values loaded from AppConfig.
func SetLogLevel(raw string) {
	level := strings.ToUpper(strings.TrimSpace(raw))
	switch level {
	case variables.Debug:
		logLevel.Store(DEBUG)
	case variables.Info, "":
		logLevel.Store(INFO)
	case variables.Warn:
		logLevel.Store(WARN)
	case variables.Error:
		logLevel.Store(ERROR)
	case variables.NoLogs:
		logLevel.Store(NOLOGS)
	default:
		logLevel.Store(INFO)
	}
}

// Error logs an error message
func Error(err error) {
	if logLevel.Load() <= ERROR {
		Logger.Println("ERROR:", err)
	}
}

// Warn logs a warning message
func Warn(message string) {
	if logLevel.Load() <= WARN {
		Logger.Println("WARN:", message)
	}
}

// Info logs an informational message
func Info(message string) {
	if logLevel.Load() <= INFO {
		Logger.Println("INFO:", message)
	}
}

// Debug logs a debug message
func Debug(message string) {
	if logLevel.Load() <= DEBUG {
		Logger.Println("DEBUG:", message)
	}
}
