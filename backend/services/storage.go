package services

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

func NewStorage(dbPath string) (*Storage, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrent read/write performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Storage{db: db}, nil
}

func runMigrations(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS clip_embeddings (
    id           TEXT PRIMARY KEY,
    embedding    BLOB NOT NULL,
    camera_id    TEXT NOT NULL,
    timestamp    TEXT NOT NULL,
    frame_path   TEXT NOT NULL,
    source_video TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_clip_camera_ts ON clip_embeddings(camera_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_clip_created ON clip_embeddings(created_at);

CREATE TABLE IF NOT EXISTS face_embeddings (
    id           TEXT PRIMARY KEY,
    embedding    BLOB NOT NULL,
    camera_id    TEXT NOT NULL,
    timestamp    TEXT NOT NULL,
    frame_path   TEXT NOT NULL,
    bbox_top     INTEGER NOT NULL,
    bbox_right   INTEGER NOT NULL,
    bbox_bottom  INTEGER NOT NULL,
    bbox_left    INTEGER NOT NULL,
    person_name  TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_face_camera_ts ON face_embeddings(camera_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_face_person ON face_embeddings(person_name);
CREATE INDEX IF NOT EXISTS idx_face_created ON face_embeddings(created_at);
`
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

func (s *Storage) AddClipEmbedding(id string, embedding []byte,
	cameraID, timestamp, framePath, sourceVideo string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO clip_embeddings
		(id, embedding, camera_id, timestamp, frame_path, source_video)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, embedding, cameraID, timestamp, framePath, sourceVideo)
	return err
}

func (s *Storage) Cleanup(olderThan time.Time) (int64, error) {
	ts := olderThan.Format(time.RFC3339)
	var total int64

	res, err := s.db.Exec("DELETE FROM clip_embeddings WHERE created_at < ?", ts)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	total += n

	res, err = s.db.Exec(
		"DELETE FROM face_embeddings WHERE created_at < ? AND person_name IS NULL",
		ts)
	if err != nil {
		return total, err
	}
	n, _ = res.RowsAffected()
	total += n

	return total, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// Float64sToBytes converts a slice of float64 values (from JSON) to raw
// little-endian float32 bytes, matching Python's np.frombuffer(blob, dtype=np.float32).
func Float64sToBytes(vals []float64) []byte {
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(float32(v)))
	}
	return buf
}
