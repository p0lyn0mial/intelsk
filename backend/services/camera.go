package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/models"
)

var validCameraID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type CameraService struct {
	db  *sql.DB
	cfg *config.AppConfig
}

func NewCameraService(db *sql.DB, cfg *config.AppConfig) *CameraService {
	return &CameraService{db: db, cfg: cfg}
}

// List returns cameras merged from the DB and filesystem.
// Filesystem-only cameras (directories with no DB row) appear as type "local".
func (s *CameraService) List() ([]models.CameraInfo, error) {
	// 1. Load all DB cameras
	dbCameras := make(map[string]models.CameraInfo)
	rows, err := s.db.Query("SELECT id, name, type, config, created_at, updated_at FROM cameras")
	if err != nil {
		return nil, fmt.Errorf("querying cameras: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cam models.CameraInfo
		var configJSON string
		if err := rows.Scan(&cam.ID, &cam.Name, &cam.Type, &configJSON, &cam.CreatedAt, &cam.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning camera row: %w", err)
		}
		if err := json.Unmarshal([]byte(configJSON), &cam.Config); err != nil {
			cam.Config = map[string]any{}
		}
		cam.Status = s.computeStatus(cam.ID)
		dbCameras[cam.ID] = cam
	}

	cameras := make([]models.CameraInfo, 0, len(dbCameras))
	for _, cam := range dbCameras {
		cameras = append(cameras, cam)
	}
	return cameras, nil
}

// Get returns a single camera by ID, checking DB first then filesystem.
func (s *CameraService) Get(id string) (*models.CameraInfo, error) {
	var cam models.CameraInfo
	var configJSON string
	err := s.db.QueryRow(
		"SELECT id, name, type, config, created_at, updated_at FROM cameras WHERE id = ?", id,
	).Scan(&cam.ID, &cam.Name, &cam.Type, &configJSON, &cam.CreatedAt, &cam.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("camera not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying camera: %w", err)
	}

	if err := json.Unmarshal([]byte(configJSON), &cam.Config); err != nil {
		cam.Config = map[string]any{}
	}
	cam.Status = s.computeStatus(cam.ID)
	return &cam, nil
}

// Create validates and inserts a new camera, creating its video directory.
func (s *CameraService) Create(req models.CreateCameraRequest) (*models.CameraInfo, error) {
	if !validCameraID.MatchString(req.ID) {
		return nil, fmt.Errorf("invalid camera ID: must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,63}")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Type != "local" && req.Type != "test" {
		return nil, fmt.Errorf("type must be 'local' or 'test'")
	}

	// Check for duplicates (DB or filesystem)
	if _, err := s.Get(req.ID); err == nil {
		return nil, fmt.Errorf("camera already exists: %s", req.ID)
	}

	config := req.Config
	if config == nil {
		config = map[string]any{}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}

	_, err = s.db.Exec(
		"INSERT INTO cameras (id, name, type, config) VALUES (?, ?, ?, ?)",
		req.ID, req.Name, req.Type, string(configJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting camera: %w", err)
	}

	// Create video directory
	videosDir := filepath.Join(s.cfg.App.DataDir, "videos", req.ID)
	if err := os.MkdirAll(videosDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating video directory: %w", err)
	}

	return s.Get(req.ID)
}

// Update modifies a camera's name and/or config.
func (s *CameraService) Update(id string, req models.UpdateCameraRequest) (*models.CameraInfo, error) {
	// Verify camera exists in DB
	var existing string
	err := s.db.QueryRow("SELECT id FROM cameras WHERE id = ?", id).Scan(&existing)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("camera not found in database: %s (filesystem-only cameras cannot be edited)", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying camera: %w", err)
	}

	if req.Name != "" {
		if _, err := s.db.Exec("UPDATE cameras SET name = ?, updated_at = datetime('now') WHERE id = ?", req.Name, id); err != nil {
			return nil, fmt.Errorf("updating name: %w", err)
		}
	}
	if req.Config != nil {
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("marshaling config: %w", err)
		}
		if _, err := s.db.Exec("UPDATE cameras SET config = ?, updated_at = datetime('now') WHERE id = ?", string(configJSON), id); err != nil {
			return nil, fmt.Errorf("updating config: %w", err)
		}
	}

	return s.Get(id)
}

// Delete removes a camera from the DB and optionally cleans up all associated data.
func (s *CameraService) Delete(id string, deleteData bool) error {
	// Delete DB row (may not exist for filesystem-only cameras)
	s.db.Exec("DELETE FROM cameras WHERE id = ?", id)

	if deleteData {
		// Remove video files
		videosDir := filepath.Join(s.cfg.App.DataDir, "videos", id)
		os.RemoveAll(videosDir)

		// Remove extracted frames
		framesDir := filepath.Join(s.cfg.Extraction.StoragePath, id)
		os.RemoveAll(framesDir)

		// Remove embeddings from DB
		s.db.Exec("DELETE FROM clip_embeddings WHERE camera_id = ?", id)
		s.db.Exec("DELETE FROM face_embeddings WHERE camera_id = ?", id)

		// Remove from process history
		s.removeFromProcessHistory(id)
	}

	return nil
}

// Download fetches a video from a URL and saves it to the camera's directory.
// Only works for cameras with type "test".
func (s *CameraService) Download(id string, url string) (string, error) {
	cam, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if cam.Type != "test" {
		return "", fmt.Errorf("download is only available for test cameras")
	}

	// Create dated subdirectory
	today := time.Now().Format("2006-01-02")
	dir := filepath.Join(s.cfg.App.DataDir, "videos", id, today)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	filename := fmt.Sprintf("download_%d.mp4", time.Now().Unix())
	destPath := filepath.Join(dir, filename)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("URL returned status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("downloading video: %w", err)
	}

	return destPath, nil
}

// Upload saves an uploaded file to the camera's video directory.
// Only works for cameras with type "local".
func (s *CameraService) Upload(id string, file io.Reader, filename string) (string, error) {
	cam, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if cam.Type != "local" {
		return "", fmt.Errorf("upload is only available for local cameras")
	}

	// Create dated subdirectory
	today := time.Now().Format("2006-01-02")
	dir := filepath.Join(s.cfg.App.DataDir, "videos", id, today)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	// Sanitize filename: keep only the base name, replace unsafe chars
	safe := filepath.Base(filename)
	safe = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, safe)
	if safe == "" || safe == "." || safe == ".." {
		safe = "upload.mp4"
	}

	// Handle filename collision by appending _1, _2, etc.
	destPath := filepath.Join(dir, safe)
	if _, err := os.Stat(destPath); err == nil {
		ext := filepath.Ext(safe)
		base := strings.TrimSuffix(safe, ext)
		for i := 1; ; i++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				destPath = candidate
				break
			}
		}
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("saving file: %w", err)
	}

	return destPath, nil
}

func (s *CameraService) computeStatus(cameraID string) string {
	framesDir := filepath.Join(s.cfg.Extraction.StoragePath, cameraID)
	dateEntries, err := os.ReadDir(framesDir)
	if err != nil {
		return "offline"
	}
	for _, de := range dateEntries {
		if de.IsDir() {
			manifest := filepath.Join(framesDir, de.Name(), "manifest.json")
			if _, err := os.Stat(manifest); err == nil {
				return "indexed"
			}
		}
	}
	return "offline"
}

func (s *CameraService) removeFromProcessHistory(cameraID string) {
	historyPath := s.cfg.Process.HistoryPath
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return
	}

	var history []models.ProcessHistoryEntry
	if err := json.Unmarshal(data, &history); err != nil {
		return
	}

	filtered := make([]models.ProcessHistoryEntry, 0, len(history))
	for _, h := range history {
		if h.CameraID != cameraID {
			filtered = append(filtered, h)
		}
	}

	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(historyPath, out, 0o644)
}
