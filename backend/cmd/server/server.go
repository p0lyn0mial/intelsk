package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/intelsk/backend/api"
	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/services"
)

func Start(cfg *config.AppConfig) {
	// Init ML client
	mlClient := services.NewMLClient(cfg.MLService.URL)
	log.Printf("ML sidecar configured at %s", cfg.MLService.URL)

	// Init storage
	storage, err := services.NewStorage(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("opening storage: %v", err)
	}
	defer storage.Close()
	log.Printf("SQLite storage at %s", cfg.Storage.DBPath)

	// Init settings
	settingsSvc := services.NewSettingsService(storage.DB(), cfg)

	// Init services
	cameraSvc := services.NewCameraService(storage.DB(), cfg)

	// Init handlers
	processHandler := api.NewProcessHandler(cfg, mlClient, storage, settingsSvc)
	searchHandler := api.NewSearchHandler(cfg, mlClient, settingsSvc)
	camerasHandler := api.NewCamerasHandler(cameraSvc)
	videoHandler := api.NewVideoHandler(cfg)
	settingsHandler := api.NewSettingsHandler(settingsSvc)

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			api.HealthCheck(w, r, mlClient)
		})

		// Process pipeline
		r.Post("/process", processHandler.Start)
		r.Get("/process/status", processHandler.Status)
		r.Get("/process/history", processHandler.History)

		// Search
		r.Post("/search/text", searchHandler.TextSearch)

		// Settings
		r.Get("/settings", settingsHandler.Get)
		r.Put("/settings", settingsHandler.Update)

		// Cameras
		r.Get("/cameras", camerasHandler.List)
		r.Get("/cameras/{id}", camerasHandler.Get)
		r.Post("/cameras", camerasHandler.Create)
		r.Put("/cameras/{id}", camerasHandler.Update)
		r.Delete("/cameras/{id}", camerasHandler.Delete)
		r.Get("/cameras/{id}/stats", camerasHandler.Stats)
		r.Get("/cameras/{id}/videos", camerasHandler.ListVideos)
		r.Delete("/cameras/{id}/videos", camerasHandler.DeleteVideo)
		r.Delete("/cameras/{id}/data", camerasHandler.CleanData)
		r.Post("/cameras/{id}/upload", camerasHandler.Upload)

		// Video playback
		r.Get("/videos/{video_id}/play", videoHandler.Play)

		// Static frame serving with path traversal protection
		r.Get("/frames/*", serveFrames(cfg))
	})

	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func serveFrames(cfg *config.AppConfig) http.HandlerFunc {
	framesDir := cfg.Extraction.StoragePath
	absFramesDir, _ := filepath.Abs(framesDir)

	return func(w http.ResponseWriter, r *http.Request) {
		// Extract path after /api/frames/
		framePath := chi.URLParam(r, "*")
		if framePath == "" {
			http.Error(w, "frame path required", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(framesDir, framePath)

		// Path traversal protection
		absPath, err := filepath.Abs(filePath)
		if err != nil || !strings.HasPrefix(absPath, absFramesDir) {
			http.Error(w, "invalid frame path", http.StatusBadRequest)
			return
		}

		http.ServeFile(w, r, absPath)
	}
}
