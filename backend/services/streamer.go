package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Streamer manages on-demand ffmpeg processes for RTSP → HLS transcoding.
type Streamer struct {
	streams map[string]*stream
	mu      sync.Mutex
	baseDir string // e.g., data/streams/
}

type stream struct {
	cmd        *exec.Cmd
	dir        string
	lastAccess time.Time
}

func NewStreamer(baseDir string) *Streamer {
	os.MkdirAll(baseDir, 0o755)
	return &Streamer{
		streams: make(map[string]*stream),
		baseDir: baseDir,
	}
}

// Start spawns an ffmpeg process to transcode RTSP to HLS for the given camera.
func (s *Streamer) Start(cameraID, rtspURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Already running?
	if st, ok := s.streams[cameraID]; ok {
		st.lastAccess = time.Now()
		return nil
	}

	dir := filepath.Join(s.baseDir, cameraID)
	os.MkdirAll(dir, 0o755)

	playlist := filepath.Join(dir, "index.m3u8")
	cmd := exec.Command(
		"ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "5",
		"-hls_flags", "delete_segments",
		"-y",
		playlist,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	log.Printf("Stream started for camera %s (pid %d)", cameraID, cmd.Process.Pid)

	s.streams[cameraID] = &stream{
		cmd:        cmd,
		dir:        dir,
		lastAccess: time.Now(),
	}

	// Reap process when it exits
	go func() {
		cmd.Wait()
		s.mu.Lock()
		if st, ok := s.streams[cameraID]; ok && st.cmd == cmd {
			delete(s.streams, cameraID)
		}
		s.mu.Unlock()
	}()

	return nil
}

// Stop kills the ffmpeg process and removes the temp directory.
func (s *Streamer) Stop(cameraID string) error {
	s.mu.Lock()
	st, ok := s.streams[cameraID]
	if ok {
		delete(s.streams, cameraID)
	}
	s.mu.Unlock()

	if !ok {
		return nil
	}

	if st.cmd.Process != nil {
		st.cmd.Process.Kill()
	}
	os.RemoveAll(st.dir)
	log.Printf("Stream stopped for camera %s", cameraID)
	return nil
}

// Dir returns the HLS directory path for a camera's stream.
func (s *Streamer) Dir(cameraID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.streams[cameraID]; ok {
		return st.dir
	}
	return ""
}

// Touch updates the lastAccess timestamp for a camera's stream.
func (s *Streamer) Touch(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.streams[cameraID]; ok {
		st.lastAccess = time.Now()
	}
}

// IsActive returns whether a stream is running for the given camera.
func (s *Streamer) IsActive(cameraID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.streams[cameraID]
	return ok
}

// StartCleanup runs a background goroutine that kills streams idle for more than 30s.
func (s *Streamer) StartCleanup() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanIdle()
		}
	}()
}

func (s *Streamer) cleanIdle() {
	s.mu.Lock()
	var toStop []string
	for id, st := range s.streams {
		if time.Since(st.lastAccess) > 30*time.Second {
			toStop = append(toStop, id)
		}
	}
	s.mu.Unlock()

	for _, id := range toStop {
		log.Printf("Stopping idle stream for camera %s", id)
		s.Stop(id)
	}
}

// StopAll stops all active streams (for graceful shutdown).
func (s *Streamer) StopAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.streams))
	for id := range s.streams {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.Stop(id)
	}
}
