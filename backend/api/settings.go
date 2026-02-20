package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type SettingsHandler struct {
	settings *services.SettingsService
	cfg      *config.AppConfig
	mlClient *services.MLClient
	storage  *services.Storage
}

func NewSettingsHandler(settings *services.SettingsService, cfg *config.AppConfig, mlClient *services.MLClient, storage *services.Storage) *SettingsHandler {
	return &SettingsHandler{
		settings: settings,
		cfg:      cfg,
		mlClient: mlClient,
		storage:  storage,
	}
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.SettingsResponse{
		Settings: h.settings.All(),
		Defaults: h.settings.Defaults(),
	})
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req models.SettingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var errors []string
	for key, value := range req.Settings {
		if err := h.settings.Set(key, value); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", key, err))
		}
	}

	if len(errors) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors":   errors,
			"settings": h.settings.All(),
			"defaults": h.settings.Defaults(),
		})
		return
	}

	writeJSON(w, http.StatusOK, models.SettingsResponse{
		Settings: h.settings.All(),
		Defaults: h.settings.Defaults(),
	})
}

func (h *SettingsHandler) GetClipModel(w http.ResponseWriter, r *http.Request) {
	info, err := h.mlClient.GetModelInfo()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("failed to get model info: %v", err),
		})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *SettingsHandler) SwitchClipModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Preset string `json:"preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Preset == "" {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Reload model in ML sidecar (downloads weights if needed)
	info, err := h.mlClient.ReloadModel(req.Preset)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("failed to reload model: %v", err),
		})
		return
	}

	// Clear all clip embeddings
	if _, err := h.storage.DB().Exec("DELETE FROM clip_embeddings"); err != nil {
		log.Printf("warning: failed to clear clip_embeddings: %v", err)
	}

	// Delete all index_state.json files under storage path
	filepath.Walk(h.cfg.Extraction.StoragePath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.Name() == "index_state.json" {
			os.Remove(path)
		}
		return nil
	})

	// Clear process history
	if h.cfg.Process.HistoryPath != "" {
		os.Remove(h.cfg.Process.HistoryPath)
	}

	// Update clip.model setting
	if err := h.settings.Set("clip.model", req.Preset); err != nil {
		log.Printf("warning: failed to update clip.model setting: %v", err)
	}

	writeJSON(w, http.StatusOK, info)
}

// NVRStatus tests connectivity to the NVR using the current settings.
func (h *SettingsHandler) NVRStatus(w http.ResponseWriter, r *http.Request) {
	ip := h.settings.Get("nvr.ip")
	if ip == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "not_configured"})
		return
	}

	username := h.settings.Get("nvr.username")
	password := h.settings.Get("nvr.password")

	client := services.NewHikvisionClient(ip, username, password)
	if err := client.Ping(); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}
