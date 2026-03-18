package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/windfall/uwu_service/internal/domain/auth"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

type contextKey string

const UserIDKey contextKey = "user_id"

// Auth returns a middleware that validates JWT tokens from the Authorization header.
func Auth(authRepo auth.AuthRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.HandleError(w, errors.Unauthorized("missing authorization header"))
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.HandleError(w, errors.Unauthorized("invalid authorization format"))
				return
			}

			tokenClaims, err := authRepo.ValidateToken(parts[1])
			if err != nil {
				response.HandleError(w, errors.Unauthorized("invalid or expired token"))
				return
			}

			// Set user ID in context
			ctx := context.WithValue(r.Context(), UserIDKey, tokenClaims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}
