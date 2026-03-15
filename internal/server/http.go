package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/config"
	handler "github.com/windfall/uwu_service/internal/handler"
	httphandler "github.com/windfall/uwu_service/internal/handler/http"
	"github.com/windfall/uwu_service/internal/middleware"
	"github.com/windfall/uwu_service/internal/service"
)

// HTTPServer represents the HTTP server.
type HTTPServer struct {
	server *http.Server
	log    zerolog.Logger
}

// NewHTTPServer creates a new HTTP server.
func NewHTTPServer(
	cfg *config.Config,
	log zerolog.Logger,
	healthHandler *httphandler.HealthHandler,
	apiHandler *httphandler.APIHandler,

	speakingHandler *httphandler.SpeakingHandler,
	learningItemHandler *httphandler.LearningItemHandler,
	authHandler *httphandler.AuthHandler,
	authService *service.AuthService,
	videoHandler *httphandler.VideoHandler,
	quizHandler *httphandler.QuizHandler,
	retellHandler *httphandler.RetellHandler,
	workoutHandler *httphandler.WorkoutHandler,
	userStatsHandler *httphandler.UserStatsHandler,

	userContentHandler *handler.UserContentHandler,
) *HTTPServer {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger(log))
	r.Use(middleware.Recovery(log))
	r.Use(chimiddleware.Compress(5))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health endpoints (public)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)
	r.Get("/live", healthHandler.Live)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoints
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected endpoints (require JWT)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(authService))

			// ---------------------
			// 	Current API
			// ---------------------

			// Learning content endpoints
			// GET /learning-items/{feature}
			// POST /learning-items/upload-video
			// GET /learning-items/upload-video/{batchID}
			// POST /learning-items/generate-dialog
			// GET /learning-items/generate-dialog/{batchID}

			// User action endpoints
			// POST /user-actions/type/{actionType}
			// GET /user-actions/summaries

			// User Content endpoints
			r.Get("/user-contents/items", userContentHandler.GetContentsByFeature)
			r.Get("/user-contents/summaries", userStatsHandler.GetLearningSummary)
			// Post/user-contents/actions: dialogue_saved, quiz_saved, quiz_transcript, submit_chat, submit_speech

			// Dialogue Guide endpoints
			r.Post("/dialogue-guides/generate", apiHandler.GenerateDialogueGuide)
			r.Get("/dialogue-guides/generate/{batchID}", apiHandler.GetGenerateProgress)
			r.Post("/dialogue-guides/submit-speech", apiHandler.SubmitDialogueSpeech)
			r.Post("/dialogue-guides/submit-chat", apiHandler.SubmitDialogueChat)

			// Native Video endpoints
			r.Post("/native-videos/upload", videoHandler.UploadNativeVideo)
			r.Get("/native-videos/upload/{batchID}", videoHandler.GetUploadProgress)
			// POST/native-videos/submit-quiz

			// ---------------------
			//   Legacy API
			// ---------------------

			// AI endpoints
			r.Post("/ai/chat", apiHandler.Chat)
			r.Post("/ai/complete", apiHandler.Complete)

			// Speech endpoints
			r.Post("/speech/analyze/vocab", apiHandler.AnalyzeVocab)
			r.Post("/speech/analyze/shadowing", apiHandler.AnalyzeShadowing)

			// Vocab endpoints
			r.Get("/vocab/mock", apiHandler.GetMockVocab)

			// Shadowing endpoints
			r.Get("/shadowing/mock", apiHandler.GetMockShadowing)

			// Speaking async endpoints (2-step pattern)
			r.Post("/speaking/analyze", speakingHandler.Analyze)
			r.Get("/speaking/reply", speakingHandler.GetReply)

			// Learning Items endpoints
			r.Post("/learning-items", learningItemHandler.CreateLearningItem)
			r.Get("/learning-items", learningItemHandler.ListLearningItems)
			r.Get("/learning-items/{id}", learningItemHandler.GetLearningItem)
			r.Put("/learning-items/{id}", learningItemHandler.UpdateLearningItem)
			r.Delete("/learning-items/{id}", learningItemHandler.DeleteLearningItem)
			r.Post("/learning-items/actions", learningItemHandler.CreateUserAction)

			// Conversation Scenarios endpoints
			r.Post("/conversation-scenarios", apiHandler.CreateConversationScenario)
			r.Get("/conversation-scenarios/{id}", apiHandler.GetConversationScenario)

			// Video endpoints
			r.Get("/videos/playlist", videoHandler.GetVideoPlaylist)
			r.Get("/videos/{videoID}", videoHandler.Get)
			// Quiz grading endpoint
			r.Post("/quiz/{lessonID}/grade", quizHandler.Grade)

			// Retell check endpoints
			r.Post("/quiz/{lessonID}/retell", retellHandler.SubmitAttempt)
			r.Get("/quiz/{lessonID}/retell", retellHandler.GetStatus)
			r.Post("/quiz/{lessonID}/retell/reset", retellHandler.Reset)

			// Workout endpoints
			r.Post("/workouts/generate", workoutHandler.Generate)
			r.Post("/workouts/pre-brief", workoutHandler.GeneratePreBrief)
			r.Post("/workouts/conversation", workoutHandler.GenerateConversation)
			r.Post("/workouts/learning-items", workoutHandler.GenerateLearningItems)
			r.Get("/workouts/batches/{batchID}", workoutHandler.GetBatchStatus)

			// Batch status endpoint
			r.Get("/batches/{batchID}", videoHandler.GetBatchStatus)

		})
	})

	server := &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &HTTPServer{
		server: server,
		log:    log,
	}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start() error {
	s.log.Info().Str("addr", s.server.Addr).Msg("Starting HTTP server")
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.log.Info().Msg("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}
