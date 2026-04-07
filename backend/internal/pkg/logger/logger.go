package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New creates a configured zerolog logger.
func New(level string) zerolog.Logger {
	// Parse log level
	var lvl zerolog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = zerolog.DebugLevel
	case "warn", "warning":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	// Console writer for readable output
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	return zerolog.New(output).
		Level(lvl).
		With().
		Timestamp().
		Caller().
		Logger()
}
