package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// 🔑 LOCAL ExtendedLogger interface definition (matches the one used by external package)
type ExtendedLogger interface {
	// Core MCP-Go compatibility methods
	Infof(format string, v ...any)
	Errorf(format string, v ...any)

	// Additional methods we need
	Info(args ...interface{})
	Error(args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})

	// Structured logging methods
	WithField(key string, value interface{}) *logrus.Entry
	WithFields(fields logrus.Fields) *logrus.Entry
	WithError(err error) *logrus.Entry

	// File management
	Close() error
}

// CustomLogger writes logs to file instead of console and implements ExtendedLogger interface
type CustomLogger struct {
	file   *os.File
	prefix string
}

var sharedLogger *CustomLogger

// InitLogger initializes the shared logger
func InitLogger(prefix string) error {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %v", err)
	}

	// Create custom log file
	file, err := os.OpenFile("logs/mcp-agent.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %v", err)
	}

	sharedLogger = &CustomLogger{
		file:   file,
		prefix: prefix,
	}

	return nil
}

// GetLogger returns the shared logger instance
func GetLogger() ExtendedLogger {
	if sharedLogger == nil {
		// Fallback to console if logger not initialized
		return &CustomLogger{
			file:   nil,
			prefix: "FALLBACK",
		}
	}
	return sharedLogger
}

// CloseLogger closes the shared logger
func CloseLogger() error {
	if sharedLogger != nil && sharedLogger.file != nil {
		return sharedLogger.file.Close()
	}
	return nil
}

func (l *CustomLogger) logToFile(level, message string) {
	if l.file == nil {
		// Fallback to console if file is not available
		fmt.Printf("[%s][%s][%s] %s\n", l.prefix, level, time.Now().Format("15:04:05.000"), message)
		return
	}

	timestamp := time.Now().Format("15:04:05.000")
	logEntry := fmt.Sprintf("[%s][%s][%s] %s\n", l.prefix, level, timestamp, message)
	l.file.WriteString(logEntry)
}

// 🔑 FIXED: Info method now accepts ...interface{} to match ExtendedLogger interface
func (l *CustomLogger) Info(args ...interface{}) {
	l.logToFile("INFO", fmt.Sprint(args...))
}

// 🔑 FIXED: Error method now accepts ...interface{} to match ExtendedLogger interface
func (l *CustomLogger) Error(args ...interface{}) {
	l.logToFile("ERROR", fmt.Sprint(args...))
}

func (l *CustomLogger) Infof(format string, args ...interface{}) {
	l.logToFile("INFO", fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Errorf(format string, args ...interface{}) {
	l.logToFile("ERROR", fmt.Sprintf(format, args...))
}

// Additional methods needed for ExtendedLogger interface
func (l *CustomLogger) Debug(args ...interface{}) {
	l.logToFile("DEBUG", fmt.Sprint(args...))
}

func (l *CustomLogger) Debugf(format string, args ...interface{}) {
	l.logToFile("DEBUG", fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Warn(args ...interface{}) {
	l.logToFile("WARN", fmt.Sprint(args...))
}

func (l *CustomLogger) Warnf(format string, args ...interface{}) {
	l.logToFile("WARN", fmt.Sprintf(format, args...))
}

func (l *CustomLogger) Fatal(args ...interface{}) {
	l.logToFile("FATAL", fmt.Sprint(args...))
}

func (l *CustomLogger) Fatalf(format string, args ...interface{}) {
	l.logToFile("FATAL", fmt.Sprintf(format, args...))
}

// Structured logging methods
func (l *CustomLogger) WithField(key string, value interface{}) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	entry.Data = map[string]interface{}{key: value}
	return entry
}

// 🔑 FIXED: WithFields method now uses logrus.Fields to match ExtendedLogger interface
func (l *CustomLogger) WithFields(fields logrus.Fields) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	entry.Data = fields
	return entry
}

func (l *CustomLogger) WithError(err error) *logrus.Entry {
	// Create a minimal logrus entry that won't output to console
	entry := logrus.NewEntry(logrus.New())
	return entry.WithError(err)
}

func (l *CustomLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
