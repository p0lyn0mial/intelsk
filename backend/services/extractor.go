package services

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/intelsk/backend/models"
)

// ExtractFramesTime runs ffmpeg to extract frames at a fixed interval from a video file.
// It parses the video path to derive camera_id, date, and segment hour.
// Returns a slice of FrameMetadata for each extracted frame.
func ExtractFramesTime(videoPath, outputDir string, intervalSec, quality int) ([]models.FrameMetadata, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	// Use segment-specific prefix (e.g. frame_0800_000001.jpg) to avoid
	// collisions when multiple videos share the same output directory.
	segName := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(filepath.Base(videoPath)))
	outputPattern := filepath.Join(outputDir, fmt.Sprintf("frame_%s_%%06d.jpg", segName))

	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=1/%d", intervalSec),
		"-q:v", strconv.Itoa(quality),
		"-y",
		outputPattern,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	cameraID, segmentStart, err := parseVideoPath(videoPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("reading output dir: %w", err)
	}

	prefix := fmt.Sprintf("frame_%s_", segName)
	var frames []models.FrameMetadata
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}

		numStr := strings.TrimPrefix(e.Name(), prefix)
		numStr = strings.TrimSuffix(numStr, ".jpg")
		frameNum, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}

		// Frame 1 = first extracted frame (at time 0), frame 2 at intervalSec, etc.
		frameTimestamp := segmentStart.Add(time.Duration((frameNum - 1) * intervalSec) * time.Second)

		frames = append(frames, models.FrameMetadata{
			FramePath:        filepath.Join(outputDir, e.Name()),
			CameraID:         cameraID,
			Timestamp:        frameTimestamp,
			SourceVideo:      videoPath,
			FrameNumber:      frameNum,
			ExtractionMethod: "time",
		})
	}

	sort.Slice(frames, func(i, j int) bool {
		return frames[i].FrameNumber < frames[j].FrameNumber
	})

	return frames, nil
}

// DeduplicateFrames removes near-duplicate frames using perceptual hashing (pHash).
// Frames whose pHash distance is below the threshold are considered duplicates;
// their JPEG files are deleted and they are removed from the returned slice.
func DeduplicateFrames(frames []models.FrameMetadata, threshold int) ([]models.FrameMetadata, error) {
	if len(frames) == 0 {
		return frames, nil
	}

	type hashedFrame struct {
		meta models.FrameMetadata
		hash *goimagehash.ImageHash
	}

	var kept []hashedFrame

	for _, f := range frames {
		file, err := os.Open(f.FramePath)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", f.FramePath, err)
		}

		img, _, err := image.Decode(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %w", f.FramePath, err)
		}

		hash, err := goimagehash.PerceptionHash(img)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", f.FramePath, err)
		}

		isDup := false
		for _, k := range kept {
			dist, err := hash.Distance(k.hash)
			if err != nil {
				return nil, fmt.Errorf("comparing hashes: %w", err)
			}
			if dist < threshold {
				isDup = true
				break
			}
		}

		if isDup {
			os.Remove(f.FramePath)
		} else {
			kept = append(kept, hashedFrame{meta: f, hash: hash})
		}
	}

	result := make([]models.FrameMetadata, len(kept))
	for i, k := range kept {
		result[i] = k.meta
	}
	return result, nil
}

// WriteManifest writes the frame metadata slice as a JSON manifest file.
func WriteManifest(outputDir string, frames []models.FrameMetadata) error {
	path := filepath.Join(outputDir, "manifest.json")
	data, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadManifest reads frame metadata from an existing manifest.json.
// Returns nil, nil if no manifest exists yet.
func LoadManifest(dir string) ([]models.FrameMetadata, error) {
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var frames []models.FrameMetadata
	return frames, json.Unmarshal(data, &frames)
}

// parseVideoPath extracts camera_id and segment start time from a video path.
// Expected path structure: .../data/videos/{camera_id}/{date}/{HH}00.mp4
func parseVideoPath(videoPath string) (cameraID string, segmentStart time.Time, err error) {
	abs, err := filepath.Abs(videoPath)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("resolving path: %w", err)
	}

	parts := strings.Split(filepath.ToSlash(abs), "/")
	if len(parts) < 3 {
		return "", time.Time{}, fmt.Errorf("video path too short to parse camera/date/hour: %s", videoPath)
	}

	filename := parts[len(parts)-1]   // e.g. "0800.mp4"
	dateStr := parts[len(parts)-2]    // e.g. "2026-02-18"
	cameraID = parts[len(parts)-3]    // e.g. "front_door"

	hourStr := strings.TrimSuffix(filename, filepath.Ext(filename)) // "0800"
	if len(hourStr) < 2 {
		return "", time.Time{}, fmt.Errorf("cannot parse hour from filename: %s", filename)
	}
	hour, err := strconv.Atoi(hourStr[:2])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parsing hour from %s: %w", filename, err)
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parsing date %s: %w", dateStr, err)
	}

	segmentStart = date.Add(time.Duration(hour) * time.Hour)
	return cameraID, segmentStart, nil
}
