package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type SettingsHandler struct {
	settings *services.SettingsService
}

func NewSettingsHandler(settings *services.SettingsService) *SettingsHandler {
	return &SettingsHandler{settings: settings}
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
