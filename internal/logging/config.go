package logging

import (
	"log/slog"
	"os"
	"strings"
)

// GetLogLevel returns the log level based on the LOG_LEVEL environment variable.
// If LOG_LEVEL is not set or invalid, it defaults to Info level.
//
// Supported values (case-insensitive):
//   - DEBUG: slog.LevelDebug
//   - INFO: slog.LevelInfo
//   - WARN or WARNING: slog.LevelWarn
//   - ERROR: slog.LevelError
//
// Default: slog.LevelInfo
func GetLogLevel() slog.Level {
	levelStr := strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL")))

	switch levelStr {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		// Default to Info if not set or invalid
		return slog.LevelInfo
	}
}
