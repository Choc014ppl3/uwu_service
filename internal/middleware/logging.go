package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// Logger returns a request logging middleware.
func Logger(log zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Get request ID
			requestID := middleware.GetReqID(r.Context())

			// Process request
			next.ServeHTTP(ww, r)

			// Calculate duration
			duration := time.Since(start)

			// Log request
			log.Info().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Int("status", ww.Status()).
				Int("bytes", ww.BytesWritten()).
				Dur("duration", duration).
				Str("user_agent", r.UserAgent()).
				Msg("HTTP request")
		})
	}
}
