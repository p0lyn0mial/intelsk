package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/intelsk/backend/models"
	"github.com/intelsk/backend/services"
)

type CamerasHandler struct {
	svc *services.CameraService
}

func NewCamerasHandler(svc *services.CameraService) *CamerasHandler {
	return &CamerasHandler{svc: svc}
}

func (h *CamerasHandler) List(w http.ResponseWriter, r *http.Request) {
	cameras, err := h.svc.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cameras)
}

func (h *CamerasHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cam, err := h.svc.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CamerasHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cam, err := h.svc.Create(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, cam)
}

func (h *CamerasHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req models.UpdateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cam, err := h.svc.Update(id, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cam)
}

func (h *CamerasHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	deleteData := r.URL.Query().Get("delete_data") == "true"

	if err := h.svc.Delete(id, deleteData); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *CamerasHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stats, err := h.svc.Stats(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *CamerasHandler) ListVideos(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	videos, err := h.svc.ListVideos(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, videos)
}

func (h *CamerasHandler) CleanData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope := r.URL.Query().Get("scope")
	if scope != "videos" && scope != "all" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be 'videos' or 'all'"})
		return
	}
	if err := h.svc.CleanData(id, scope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned"})
}

func (h *CamerasHandler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	date := r.URL.Query().Get("date")
	filename := r.URL.Query().Get("filename")
	if date == "" || filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date and filename are required"})
		return
	}
	if err := h.svc.DeleteVideo(id, date, filename); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *CamerasHandler) Upload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 32 MB memory limit; rest spills to disk
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files provided"})
		return
	}

	var paths []string
	for _, fh := range files {
		// Skip non-.mp4 files
		if strings.ToLower(filepath.Ext(fh.Filename)) != ".mp4" {
			continue
		}

		file, err := fh.Open()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
			return
		}

		path, err := h.svc.Upload(id, file, fh.Filename)
		file.Close()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no .mp4 files found in upload"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "uploaded", "paths": paths})
}
