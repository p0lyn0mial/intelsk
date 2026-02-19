package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
)

type CamerasHandler struct {
	cfg *config.AppConfig
}

func NewCamerasHandler(cfg *config.AppConfig) *CamerasHandler {
	return &CamerasHandler{cfg: cfg}
}

// List returns camera IDs discovered from the data/frames/ directory structure.
func (h *CamerasHandler) List(w http.ResponseWriter, r *http.Request) {
	framesDir := h.cfg.Extraction.StoragePath
	entries, err := os.ReadDir(framesDir)
	if err != nil {
		// No frames directory yet — return empty list
		writeJSON(w, http.StatusOK, []models.CameraInfo{})
		return
	}

	var cameras []models.CameraInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		cam := models.CameraInfo{
			ID:     e.Name(),
			Name:   e.Name(),
			Status: "offline",
		}

		// Check if camera has any indexed dates (has manifest files)
		dateEntries, err := os.ReadDir(filepath.Join(framesDir, e.Name()))
		if err == nil {
			for _, de := range dateEntries {
				if de.IsDir() {
					manifest := filepath.Join(framesDir, e.Name(), de.Name(), "manifest.json")
					if _, err := os.Stat(manifest); err == nil {
						cam.Status = "indexed"
						break
					}
				}
			}
		}

		cameras = append(cameras, cam)
	}

	if cameras == nil {
		cameras = []models.CameraInfo{}
	}
	writeJSON(w, http.StatusOK, cameras)
}
