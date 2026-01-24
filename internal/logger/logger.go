package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New creates a new zerolog logger with the specified level and format.
func New(level, format string) zerolog.Logger {
	var output io.Writer = os.Stdout

	// Use pretty printing for console format
	if format == "console" || format == "text" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Parse log level
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(output).
		Level(lvl).
		With().
		Timestamp().
		Caller().
		Logger()
}

// NewWithFields creates a logger with additional fields.
func NewWithFields(level, format string, fields map[string]interface{}) zerolog.Logger {
	log := New(level, format)
	ctx := log.With()

	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}

	return ctx.Logger()
}

// NewNop creates a no-op logger for testing.
func NewNop() zerolog.Logger {
	return zerolog.Nop()
}
