package logger

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger สร้างตัวแปร Logger ตาม Level และ Format ที่กำหนด
func NewLogger(level, format string) *slog.Logger {
	var handler slog.Handler

	// กำหนดระดับของ Log (Debug, Info, Warn, Error)
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	if strings.ToLower(format) == "text" {
		// แบบ Text อ่านง่ายสำหรับนักพัฒนา
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		// แบบ JSON สำหรับ Production หรือระบบ Log Centralized
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// สร้าง Logger และกำหนดให้เป็น Default ของระบบ
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}
