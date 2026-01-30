package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"rocket4cli/internal/config"
)

// Level represents log level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Entry represents a log entry
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
}

// Logger handles logging to file and provides channel for TUI
type Logger struct {
	file     *os.File
	mu       sync.Mutex
	logCh    chan Entry
	minLevel Level
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger
func Init() error {
	var initErr error
	once.Do(func() {
		logDir, err := config.GetLogDir()
		if err != nil {
			initErr = err
			return
		}

		if err := config.EnsureDir(logDir); err != nil {
			initErr = err
			return
		}

		// Create log file with date
		logFile := filepath.Join(logDir, fmt.Sprintf("clash4cli-%s.log", time.Now().Format("2006-01-02")))
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			initErr = err
			return
		}

		defaultLogger = &Logger{
			file:     file,
			logCh:    make(chan Entry, 100),
			minLevel: LevelInfo,
		}
	})
	return initErr
}

// Close closes the logger
func Close() {
	if defaultLogger != nil && defaultLogger.file != nil {
		defaultLogger.file.Close()
	}
}

// SetLevel sets the minimum log level
func SetLevel(level Level) {
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.minLevel = level
		defaultLogger.mu.Unlock()
	}
}

// Channel returns the log channel for TUI
func Channel() <-chan Entry {
	if defaultLogger != nil {
		return defaultLogger.logCh
	}
	return nil
}

// Log writes a log entry
func Log(level Level, format string, args ...interface{}) {
	if defaultLogger == nil {
		return
	}

	defaultLogger.mu.Lock()
	if level < defaultLogger.minLevel {
		defaultLogger.mu.Unlock()
		return
	}
	defaultLogger.mu.Unlock()

	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	}

	// Write to file
	defaultLogger.writeToFile(entry)

	// Send to channel (non-blocking)
	select {
	case defaultLogger.logCh <- entry:
	default:
		// Channel full, drop the log entry for TUI
	}
}

func (l *Logger) writeToFile(entry Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	line := fmt.Sprintf("%s [%s] %s\n",
		entry.Time.Format("2006-01-02 15:04:05.000"),
		entry.Level.String(),
		entry.Message,
	)

	io.WriteString(l.file, line)
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	Log(LevelDebug, format, args...)
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	Log(LevelInfo, format, args...)
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	Log(LevelWarn, format, args...)
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	Log(LevelError, format, args...)
}
