package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/intelsk/backend/config"
)

type VideoHandler struct {
	dataDir   string
	videosDir string
}

func NewVideoHandler(cfg *config.AppConfig) *VideoHandler {
	videosDir := filepath.Join(cfg.App.DataDir, "videos")
	absVideos, _ := filepath.Abs(videosDir)
	return &VideoHandler{
		dataDir:   cfg.App.DataDir,
		videosDir: absVideos,
	}
}

// Play serves a video file by its encoded video ID.
// Video ID format: "front_door--2026-02-18--1400" → "videos/front_door/2026-02-18/1400.mp4"
// http.ServeFile handles Range requests automatically for seeking support.
func (h *VideoHandler) Play(w http.ResponseWriter, r *http.Request) {
	videoID := chi.URLParam(r, "video_id")

	// Decode video ID back to file path
	relativePath := strings.ReplaceAll(videoID, "--", "/") + ".mp4"
	filePath := filepath.Join(h.dataDir, "videos", relativePath)

	// Path validation: ensure resolved path stays within data/videos/
	absPath, err := filepath.Abs(filePath)
	if err != nil || !strings.HasPrefix(absPath, h.videosDir) {
		http.Error(w, "invalid video ID", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, absPath)
}
