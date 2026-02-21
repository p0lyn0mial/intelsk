package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
		cam.Status = s.computeStatus(cam.ID, cam.Type)
		dbCameras[cam.ID] = cam
	}

	cameras := make([]models.CameraInfo, 0, len(dbCameras))
	for _, cam := range dbCameras {
		cameras = append(cameras, cam)
	}
	sort.Slice(cameras, func(i, j int) bool {
		return cameras[i].ID < cameras[j].ID
	})
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
	cam.Status = s.computeStatus(cam.ID, cam.Type)
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
	if req.Type != "local" && req.Type != "hikvision" {
		return nil, fmt.Errorf("type must be 'local' or 'hikvision'")
	}

	// Check for duplicates (DB or filesystem)
	if _, err := s.Get(req.ID); err == nil {
		return nil, fmt.Errorf("camera already exists: %s", req.ID)
	}

	config := req.Config
	if config == nil {
		config = map[string]any{}
	}
	// Default transcode=true
	if _, ok := config["transcode"]; !ok {
		config["transcode"] = true
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

// Upload saves an uploaded file to the camera's video directory.
// Only works for cameras with type "local".
func (s *CameraService) Upload(id string, file io.Reader, filename string) (string, error) {
	if _, err := s.Get(id); err != nil {
		return "", err
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

	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(destPath)
		return "", fmt.Errorf("saving file: %w", err)
	}
	out.Close()

	return destPath, nil
}

// ProbeVideoCodec runs ffprobe and returns the codec name of the first video stream.
func ProbeVideoCodec(filePath string) (string, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "csv=p=0",
		filePath,
	).Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// TranscodeIfNeeded checks the video codec and transcodes HEVC to H.264 in-place.
func TranscodeIfNeeded(filePath string) error {
	codec, err := ProbeVideoCodec(filePath)
	if err != nil {
		return err
	}
	if codec != "hevc" {
		return nil
	}

	log.Printf("Transcoding HEVC video: %s", filePath)
	tmpPath := filePath + ".transcoding.mp4"
	cmd := exec.Command(
		"ffmpeg", "-i", filePath,
		"-c:v", "libx264", "-crf", "23",
		"-c:a", "aac",
		"-y", tmpPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ffmpeg transcode: %w: %s", err, string(out))
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replacing original with transcoded file: %w", err)
	}
	log.Printf("Transcode complete: %s", filePath)
	return nil
}

// ShouldTranscode returns whether transcoding is enabled for this camera config.
func ShouldTranscode(config map[string]any) bool {
	v, ok := config["transcode"]
	if !ok {
		return true // default enabled
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// ShouldProcessOnUpload returns whether automatic processing (extract + index)
// should run after uploading videos. Defaults to true.
func ShouldProcessOnUpload(config map[string]any) bool {
	v, ok := config["process_on_upload"]
	if !ok {
		return true // default enabled
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// Stats returns per-date video and frame counts for a camera.
func (s *CameraService) Stats(id string) ([]models.CameraDateStats, error) {
	dateMap := make(map[string]*models.CameraDateStats)

	// Count videos per date
	videosDir := filepath.Join(s.cfg.App.DataDir, "videos", id)
	if entries, err := os.ReadDir(videosDir); err == nil {
		for _, de := range entries {
			if !de.IsDir() {
				continue
			}
			date := de.Name()
			dateDir := filepath.Join(videosDir, date)
			files, err := os.ReadDir(dateDir)
			if err != nil {
				continue
			}
			videoCount := 0
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".mp4") {
					videoCount++
				}
			}
			if videoCount > 0 {
				if _, ok := dateMap[date]; !ok {
					dateMap[date] = &models.CameraDateStats{Date: date}
				}
				dateMap[date].VideoCount = videoCount
			}
		}
	}

	// Count frames per date from manifest.json
	framesDir := filepath.Join(s.cfg.Extraction.StoragePath, id)
	if entries, err := os.ReadDir(framesDir); err == nil {
		for _, de := range entries {
			if !de.IsDir() {
				continue
			}
			date := de.Name()
			manifestPath := filepath.Join(framesDir, date, "manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			var manifest []any
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}
			if _, ok := dateMap[date]; !ok {
				dateMap[date] = &models.CameraDateStats{Date: date}
			}
			dateMap[date].FrameCount = len(manifest)
		}
	}

	stats := make([]models.CameraDateStats, 0, len(dateMap))
	for _, s := range dateMap {
		stats = append(stats, *s)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Date > stats[j].Date // newest first
	})
	return stats, nil
}

// ListVideos returns all .mp4 files for a camera, grouped by date, sorted date-desc then filename-asc.
func (s *CameraService) ListVideos(id string) ([]models.VideoFile, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}

	files := make([]models.VideoFile, 0)
	videosDir := filepath.Join(s.cfg.App.DataDir, "videos", id)
	dateEntries, err := os.ReadDir(videosDir)
	if err != nil {
		return files, nil // no videos dir is fine
	}

	for _, de := range dateEntries {
		if !de.IsDir() {
			continue
		}
		date := de.Name()
		dateDir := filepath.Join(videosDir, date)
		videoEntries, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}
		for _, ve := range videoEntries {
			if ve.IsDir() || !strings.HasSuffix(strings.ToLower(ve.Name()), ".mp4") {
				continue
			}
			info, err := ve.Info()
			if err != nil {
				continue
			}
			files = append(files, models.VideoFile{
				Date:     date,
				Filename: ve.Name(),
				Size:     info.Size(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Date != files[j].Date {
			return files[i].Date > files[j].Date // newest date first
		}
		return files[i].Filename < files[j].Filename
	})
	return files, nil
}

// CleanData removes data for a camera. scope "videos" deletes only videos;
// scope "all" also removes frames, embeddings, and process history.
func (s *CameraService) CleanData(id string, scope string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}

	// Always delete videos
	videosDir := filepath.Join(s.cfg.App.DataDir, "videos", id)
	os.RemoveAll(videosDir)
	os.MkdirAll(videosDir, 0o755)

	if scope == "all" {
		framesDir := filepath.Join(s.cfg.Extraction.StoragePath, id)
		os.RemoveAll(framesDir)
		s.db.Exec("DELETE FROM clip_embeddings WHERE camera_id = ?", id)
		s.db.Exec("DELETE FROM face_embeddings WHERE camera_id = ?", id)
		s.removeFromProcessHistory(id)
	}

	return nil
}

// DeleteVideo removes a single video file and its associated frames and embeddings.
func (s *CameraService) DeleteVideo(id, date, filename string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}

	// Sanitize inputs to prevent path traversal
	safeDate := filepath.Base(date)
	safeFile := filepath.Base(filename)
	if safeDate != date || safeFile != filename || safeDate == ".." || safeFile == ".." {
		return fmt.Errorf("invalid date or filename")
	}

	filePath := filepath.Join(s.cfg.App.DataDir, "videos", id, safeDate, safeFile)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("video not found")
	}
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("removing video: %w", err)
	}

	// Remove frames and embeddings associated with this video
	s.cleanVideoFramesAndEmbeddings(id, safeDate, filePath)

	// Clean up empty date directory
	dateDir := filepath.Join(s.cfg.App.DataDir, "videos", id, safeDate)
	entries, err := os.ReadDir(dateDir)
	if err == nil && len(entries) == 0 {
		os.Remove(dateDir)
	}

	return nil
}

// cleanVideoFramesAndEmbeddings removes frames, manifest entries, and DB embeddings
// that were extracted from the given video file.
func (s *CameraService) cleanVideoFramesAndEmbeddings(cameraID, date, videoPath string) {
	manifestPath := filepath.Join(s.cfg.Extraction.StoragePath, cameraID, date, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}

	var manifest []models.FrameMetadata
	if err := json.Unmarshal(data, &manifest); err != nil {
		return
	}

	// Split manifest into entries to keep vs entries to remove
	kept := make([]models.FrameMetadata, 0, len(manifest))
	for _, f := range manifest {
		if f.SourceVideo == videoPath {
			// Delete the frame image file
			os.Remove(f.FramePath)
			// Delete embeddings referencing this frame
			s.db.Exec("DELETE FROM clip_embeddings WHERE frame_path = ?", f.FramePath)
			s.db.Exec("DELETE FROM face_embeddings WHERE frame_path = ?", f.FramePath)
		} else {
			kept = append(kept, f)
		}
	}

	if len(kept) == 0 {
		// No frames left for this date — remove the entire date directory
		os.RemoveAll(filepath.Join(s.cfg.Extraction.StoragePath, cameraID, date))
	} else {
		// Rewrite manifest with remaining entries
		out, err := json.MarshalIndent(kept, "", "  ")
		if err != nil {
			return
		}
		os.WriteFile(manifestPath, out, 0o644)
	}
}

func (s *CameraService) computeStatus(cameraID, cameraType string) string {
	framesDir := filepath.Join(s.cfg.Extraction.StoragePath, cameraID)
	dateEntries, err := os.ReadDir(framesDir)
	if err != nil {
		if cameraType == "hikvision" {
			return "online"
		}
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
	if cameraType == "hikvision" {
		return "online"
	}
	return "offline"
}

// CameraRTSPUrl builds the RTSP URL for a hikvision camera using NVR settings.
// The NVR IP, port, and credentials are passed in since cameras connect through the NVR.
// streamType: 1 = main stream (high res), 2 = sub stream (low res).
func CameraRTSPUrl(cam *models.CameraInfo, nvrIP string, nvrRTSPPort int, nvrUsername, nvrPassword string, streamType int) string {
	channel := NVRChannel(cam)
	return RTSPUrl(nvrIP, nvrRTSPPort, nvrUsername, nvrPassword, channel, streamType)
}

// Thumbnail returns a JPEG thumbnail for a local camera.
// It checks for a cached file first, then tries extracted frames, and
// finally falls back to extracting the first frame from a video with ffmpeg.
func (s *CameraService) Thumbnail(id string) ([]byte, error) {
	// 1. Check cache
	cacheDir := filepath.Join(s.cfg.App.DataDir, "thumbnails")
	cachePath := filepath.Join(cacheDir, id+".jpg")
	if data, err := os.ReadFile(cachePath); err == nil {
		return data, nil
	}

	// 2. Try extracted frames (newest date first)
	framesDir := filepath.Join(s.cfg.Extraction.StoragePath, id)
	if dateEntries, err := os.ReadDir(framesDir); err == nil {
		// Sort dates newest first
		sort.Slice(dateEntries, func(i, j int) bool {
			return dateEntries[i].Name() > dateEntries[j].Name()
		})
		for _, de := range dateEntries {
			if !de.IsDir() {
				continue
			}
			dateDir := filepath.Join(framesDir, de.Name())
			manifest, err := LoadManifest(dateDir)
			if err != nil || len(manifest) == 0 {
				continue
			}
			// Read the first frame JPEG
			data, err := os.ReadFile(manifest[0].FramePath)
			if err != nil {
				continue
			}
			// Cache it
			os.MkdirAll(cacheDir, 0o755)
			os.WriteFile(cachePath, data, 0o644)
			return data, nil
		}
	}

	// 3. Fallback: extract first frame from latest video via ffmpeg
	videosDir := filepath.Join(s.cfg.App.DataDir, "videos", id)
	dateEntries, err := os.ReadDir(videosDir)
	if err != nil {
		return nil, fmt.Errorf("no videos found for camera %s", id)
	}
	// Sort dates newest first
	sort.Slice(dateEntries, func(i, j int) bool {
		return dateEntries[i].Name() > dateEntries[j].Name()
	})
	for _, de := range dateEntries {
		if !de.IsDir() {
			continue
		}
		dateDir := filepath.Join(videosDir, de.Name())
		files, err := os.ReadDir(dateDir)
		if err != nil {
			continue
		}
		// Find latest .mp4 (sort descending by name)
		var mp4s []string
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".mp4") {
				mp4s = append(mp4s, filepath.Join(dateDir, f.Name()))
			}
		}
		if len(mp4s) == 0 {
			continue
		}
		sort.Sort(sort.Reverse(sort.StringSlice(mp4s)))

		// Extract first frame with ffmpeg
		os.MkdirAll(cacheDir, 0o755)
		cmd := exec.Command(
			"ffmpeg", "-i", mp4s[0],
			"-vframes", "1", "-q:v", "2",
			"-y", cachePath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("ffmpeg thumbnail extraction failed for %s: %v: %s", mp4s[0], err, string(out))
			continue
		}
		data, err := os.ReadFile(cachePath)
		if err != nil {
			return nil, fmt.Errorf("reading generated thumbnail: %w", err)
		}
		return data, nil
	}

	return nil, fmt.Errorf("no videos or frames found for camera %s", id)
}

// InvalidateThumbnail removes the cached thumbnail for a camera.
func (s *CameraService) InvalidateThumbnail(id string) {
	cachePath := filepath.Join(s.cfg.App.DataDir, "thumbnails", id+".jpg")
	os.Remove(cachePath)
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
