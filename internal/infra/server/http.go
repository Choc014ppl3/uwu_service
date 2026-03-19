package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/windfall/uwu_service/internal/infra/config"
	"github.com/windfall/uwu_service/internal/infra/middleware"

	"github.com/windfall/uwu_service/internal/domain/auth"
	"github.com/windfall/uwu_service/internal/domain/dialog"
	"github.com/windfall/uwu_service/internal/domain/profile"
	"github.com/windfall/uwu_service/internal/domain/video"
)

// HTTPServer represents the HTTP server
type HTTPServer struct {
	server *http.Server
	log    *slog.Logger
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(
	cfg *config.Config,
	log *slog.Logger,
	authRepo auth.AuthRepository,
	authHandler *auth.AuthHandler,
	videoHandler *video.VideoHandler,
	dialogHandler *dialog.DialogHandler,
	profileHandler *profile.ProfileHandler,
) *HTTPServer {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.Logger(log))
	r.Use(middleware.Recovery(log))
	r.Use(chiMiddleware.Compress(5))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health endpoints (public)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "uwu_service",
		})
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected endpoints (require JWT)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authRepo))

			// Dialog
			r.Get("/dialogs/contents", dialogHandler.ListDialogContents)
			r.Post("/dialogs/generate", dialogHandler.GenerateDialog)
			r.Get("/dialogs/{dialogID}/details", dialogHandler.GetDialogDetails)
			r.Post("/dialogs/{dialogID}/start-speech", dialogHandler.StartSpeech)
			// r.Post("dialogs/submit-speech", dialogHandler.SubmitSpeech)
			r.Post("/dialogs/{dialogID}/start-chat", dialogHandler.StartChat)
			// r.Post("dialogs/submit-chat", dialogHandler.SubmitChat)
			r.Post("/dialogs/{dialogID}/toggle-saved", dialogHandler.ToggleSaved)

			// Video
			r.Get("/videos/contents", videoHandler.ListVideoContents)
			r.Post("/videos/upload", videoHandler.UploadVideo)
			r.Get("/videos/{videoID}/details", videoHandler.GetVideoDetails)
			r.Post("/videos/{videoID}/start-quiz", videoHandler.StartQuiz)
			// r.Post("videos/submit-quiz", videoHandler.SubmitQuiz)
			// r.Post("videos/toggle-transcript", videoHandler.ToggleTranscript)
			r.Post("/videos/{videoID}/toggle-saved", videoHandler.ToggleSaved)

			// Profile
			r.Get("/profile", profileHandler.GetProfile)
			// r.Put("profile", profileHandler.UpdateProfile)
			// r.Get("profile/stats", profileHandler.GetProfileStats)

		})
	})

	server := &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &HTTPServer{server: server, log: log}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start() error {
	s.log.Info("Starting HTTP server", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.log.Info("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}
