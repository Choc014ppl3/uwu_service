package logger

import (
	"log/slog"
	"os"
)

// New สร้างตัวแปร Logger ตาม Environment
func New(env string) *slog.Logger {
	var handler slog.Handler

	// กำหนดระดับของ Log (Info, Debug, Error)
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	if env == "development" {
		opts.Level = slog.LevelDebug
		// แบบ Text อ่านง่ายสำหรับนักพัฒนา
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		// แบบ JSON สำหรับ Production ให้เครื่องมืออื่นอ่านง่าย
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// สร้าง Logger และกำหนดให้เป็น Default ของระบบ
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}
