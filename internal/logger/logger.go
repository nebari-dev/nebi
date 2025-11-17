package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init initializes the global slog logger with the specified format and level
func Init(format, level string) {
	var handler slog.Handler
	var logLevel slog.Level

	// Parse log level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
	}

	// Create handler based on format
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text", "console":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		// Default to text for development
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	// Set as default logger
	slog.SetDefault(slog.New(handler))
}
