package api

import (
	"encoding/json"
	"net/http"

	"github.com/intelsk/backend/services"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func HealthCheck(w http.ResponseWriter, r *http.Request, mlClient *services.MLClient) {
	mlStatus := "ok"
	if err := mlClient.HealthCheck(); err != nil {
		mlStatus = "unavailable"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"ml_sidecar": mlStatus,
	})
}
