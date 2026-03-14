package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery เป็น Middleware สำหรับดักจับ Panic ไม่ให้แอปพัง
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// ใช้ defer เพื่อให้ฟังก์ชันนี้ทำงานตอนท้ายสุดเสมอ (แม้จะเกิด Panic)
			defer func() {
				if err := recover(); err != nil {
					// 1. ดึง Stack Trace ออกมาดูว่าโค้ดพังที่บรรทัดไหน
					stack := string(debug.Stack())

					// 2. บันทึก Log ระดับ Error พร้อมแนบ Stack Trace
					log.ErrorContext(r.Context(), "panic recovered",
						slog.Any("error", err),
						slog.String("path", r.URL.Path),
						slog.String("stack", stack),
					)

					// 3. ตอบกลับ Client ด้วย 500 Internal Server Error (ไม่ควรพ่น Stack Trace ให้ Client เห็นเพื่อความปลอดภัย)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "Internal Server Error", "message": "Something went wrong"}`))
				}
			}()

			// ปล่อยให้ระบบทำงานตามปกติ
			next.ServeHTTP(w, r)
		})
	}
}
