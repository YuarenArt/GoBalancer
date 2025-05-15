package logging

import (
	"github.com/YuarenArt/GoBalancer/internal/config"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger defines the interface for structured logging.
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// NewLogger creates a new Logger instance based on the provided configuration.
// It supports different logger types, currently only "slog" is implemented.
func NewLogger(cfg *config.Config) Logger {
	switch cfg.LogType {
	case "slog":
		return newSlogLogger(cfg)
	default:
		panic("unsupported logger type: " + cfg.LogType)
	}
}

// SlogLogger wraps an slog.Logger to implement the Logger interface.
type SlogLogger struct {
	logger *slog.Logger
}

// newSlogLogger creates a new SlogLogger with output to either console or file.
// The output is determined by the configuration: file is prioritized if LogToFile is true,
// otherwise console is used if LogToConsole is true. If no valid output is specified, it defaults to console.
func newSlogLogger(cfg *config.Config) Logger {
	var writer io.Writer

	// Prioritize file output if LogToFile is true
	if cfg.LogToFile {
		writer = setupFileWriter(cfg.LogFile)
	} else {
		writer = os.Stdout
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{})

	return &SlogLogger{
		logger: slog.New(handler),
	}
}

// setupFileWriter creates a file writer for logging.
// It ensures the log directory exists and opens the file in append mode.
// If the file cannot be opened, it logs an error and returns os.Stdout.
func setupFileWriter(logFile string) io.Writer {
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "error", err, "path", logDir)
		return os.Stdout
	}

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Failed to open log file", "error", err, "path", logFile)
		return os.Stdout
	}

	return file
}

// Debug logs a message at Debug level with optional key-value pairs.
func (l *SlogLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.logger.Debug(msg, keysAndValues...)
}

// Info logs a message at Info level with optional key-value pairs.
func (l *SlogLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, keysAndValues...)
}

// Warn logs a message at Warn level with optional key-value pairs.
func (l *SlogLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Warn(msg, keysAndValues...)
}

// Error logs a message at Error level with optional key-value pairs.
func (l *SlogLogger) Error(msg string, keysAndValues ...interface{}) {
	l.logger.Error(msg, keysAndValues...)
}
