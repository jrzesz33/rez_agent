package logging

import (
	"log/slog"
	"os"
	"testing"
)

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     slog.Level
	}{
		{
			name:     "DEBUG level",
			envValue: "DEBUG",
			want:     slog.LevelDebug,
		},
		{
			name:     "debug level lowercase",
			envValue: "debug",
			want:     slog.LevelDebug,
		},
		{
			name:     "INFO level",
			envValue: "INFO",
			want:     slog.LevelInfo,
		},
		{
			name:     "info level lowercase",
			envValue: "info",
			want:     slog.LevelInfo,
		},
		{
			name:     "WARN level",
			envValue: "WARN",
			want:     slog.LevelWarn,
		},
		{
			name:     "WARNING level",
			envValue: "WARNING",
			want:     slog.LevelWarn,
		},
		{
			name:     "warn level lowercase",
			envValue: "warn",
			want:     slog.LevelWarn,
		},
		{
			name:     "ERROR level",
			envValue: "ERROR",
			want:     slog.LevelError,
		},
		{
			name:     "error level lowercase",
			envValue: "error",
			want:     slog.LevelError,
		},
		{
			name:     "empty string defaults to INFO",
			envValue: "",
			want:     slog.LevelInfo,
		},
		{
			name:     "invalid value defaults to INFO",
			envValue: "INVALID",
			want:     slog.LevelInfo,
		},
		{
			name:     "whitespace only defaults to INFO",
			envValue: "  ",
			want:     slog.LevelInfo,
		},
		{
			name:     "value with whitespace",
			envValue: "  DEBUG  ",
			want:     slog.LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("LOG_LEVEL")
			defer os.Setenv("LOG_LEVEL", originalValue)

			// Set test value
			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			// Test
			got := GetLogLevel()
			if got != tt.want {
				t.Errorf("GetLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLogLevel_NotSet(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("LOG_LEVEL")
	defer func() {
		if originalValue != "" {
			os.Setenv("LOG_LEVEL", originalValue)
		}
	}()

	// Unset the environment variable
	os.Unsetenv("LOG_LEVEL")

	// Should default to Info
	got := GetLogLevel()
	if got != slog.LevelInfo {
		t.Errorf("GetLogLevel() with unset LOG_LEVEL = %v, want %v", got, slog.LevelInfo)
	}
}
