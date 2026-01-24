package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog"
)

// Recovery returns a panic recovery middleware.
func Recovery(log zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error().
						Interface("panic", err).
						Str("stack", string(debug.Stack())).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Msg("Panic recovered")

					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
