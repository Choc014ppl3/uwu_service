package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter เป็นตัวหุ้มเพื่อดักจับ status code
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	// ค่าเริ่มต้นคือ 200 OK (เผื่อ Handler ลืมเขียน Status)
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true
}

// Logger เป็น Middleware บันทึกข้อมูลการเข้าใช้งาน
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := wrapResponseWriter(w)

			// ส่ง Request ให้ชิ้นต่อไปทำงาน (อาจจะเป็น Middleware ตัวอื่น หรือ Handler)
			next.ServeHTTP(wrapped, r)

			// เมื่อทำงานเสร็จ จะกลับมาทำงานตรงนี้ต่อเพื่อบันทึก Log
			latency := time.Since(start)

			log.InfoContext(r.Context(), "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.status),
				slog.Duration("latency", latency),
				slog.String("ip", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)
		})
	}
}
