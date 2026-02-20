package services

import (
	"database/sql"
	"fmt"
	"strconv"
	"sync"

	"github.com/intelsk/backend/config"
)

type settingDef struct {
	Key      string
	Type     string // "float", "int", "bool"
	Default  string
	Min      float64
	Max      float64
}

var settingDefs = []settingDef{
	{"general.system_name", "string", "CCTV Intelligence", 0, 0},
	{"search.min_score", "float", "0.18", 0.0, 1.0},
	{"search.default_limit", "int", "20", 1, 500},
	{"extraction.time_interval_sec", "int", "5", 1, 3600},
	{"extraction.output_quality", "int", "85", 1, 100},
	{"extraction.dedup_enabled", "bool", "true", 0, 0},
	{"extraction.dedup_phash_threshold", "int", "8", 0, 64},
	{"clip.batch_size", "int", "32", 1, 256},
	{"clip.model", "string", "mobileclip-s0", 0, 0},
	{"nvr.ip", "string", "", 0, 0},
	{"nvr.port", "int", "443", 1, 65535},
	{"nvr.rtsp_port", "int", "554", 1, 65535},
	{"nvr.username", "string", "", 0, 0},
	{"nvr.password", "string", "", 0, 0},
}

type SettingsService struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]string
	defs  map[string]settingDef
}

func NewSettingsService(db *sql.DB, cfg *config.AppConfig) *SettingsService {
	s := &SettingsService{
		db:    db,
		cache: make(map[string]string),
		defs:  make(map[string]settingDef),
	}

	for _, d := range settingDefs {
		s.defs[d.Key] = d
	}

	// Build defaults from config values
	s.cache["general.system_name"] = "CCTV Intelligence"
	s.cache["search.min_score"] = "0.18"
	s.cache["search.default_limit"] = "20"
	s.cache["extraction.time_interval_sec"] = strconv.Itoa(cfg.Extraction.TimeIntervalSec)
	s.cache["extraction.output_quality"] = strconv.Itoa(cfg.Extraction.OutputQuality)
	s.cache["extraction.dedup_enabled"] = strconv.FormatBool(cfg.Extraction.DedupEnabled)
	s.cache["extraction.dedup_phash_threshold"] = strconv.Itoa(cfg.Extraction.DedupPHashThreshold)
	s.cache["clip.batch_size"] = strconv.Itoa(cfg.CLIP.BatchSize)
	s.cache["clip.model"] = "mobileclip-s0"
	s.cache["nvr.ip"] = ""
	s.cache["nvr.port"] = "443"
	s.cache["nvr.rtsp_port"] = "554"
	s.cache["nvr.username"] = ""
	s.cache["nvr.password"] = ""

	// Seed DB with defaults for any keys not yet persisted, then load all
	s.seedDefaults()
	s.loadFromDB()

	return s
}

// seedDefaults writes default values into the DB for any settings that don't
// have a row yet, so every setting is always persisted.
func (s *SettingsService) seedDefaults() {
	for key, val := range s.cache {
		s.db.Exec(
			`INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))`,
			key, val,
		)
	}
}

func (s *SettingsService) loadFromDB() {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		if _, ok := s.defs[key]; ok {
			s.cache[key] = value
		}
	}
}

func (s *SettingsService) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key]
}

func (s *SettingsService) GetFloat64(key string) float64 {
	v, _ := strconv.ParseFloat(s.Get(key), 64)
	return v
}

func (s *SettingsService) GetInt(key string) int {
	v, _ := strconv.Atoi(s.Get(key))
	return v
}

func (s *SettingsService) GetBool(key string) bool {
	v, _ := strconv.ParseBool(s.Get(key))
	return v
}

func (s *SettingsService) Set(key string, value any) error {
	def, ok := s.defs[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}

	strVal, err := s.validate(def, value)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %w", key, err)
	}

	// Persist to DB
	_, err = s.db.Exec(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, strVal,
	)
	if err != nil {
		return fmt.Errorf("saving setting %s: %w", key, err)
	}

	// Update cache
	s.mu.Lock()
	s.cache[key] = strVal
	s.mu.Unlock()

	return nil
}

func (s *SettingsService) validate(def settingDef, value any) (string, error) {
	switch def.Type {
	case "float":
		v, err := toFloat64(value)
		if err != nil {
			return "", fmt.Errorf("expected float: %w", err)
		}
		if v < def.Min || v > def.Max {
			return "", fmt.Errorf("must be between %g and %g", def.Min, def.Max)
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case "int":
		v, err := toInt(value)
		if err != nil {
			return "", fmt.Errorf("expected int: %w", err)
		}
		if float64(v) < def.Min || float64(v) > def.Max {
			return "", fmt.Errorf("must be between %d and %d", int(def.Min), int(def.Max))
		}
		return strconv.Itoa(v), nil
	case "bool":
		v, err := toBool(value)
		if err != nil {
			return "", fmt.Errorf("expected bool: %w", err)
		}
		return strconv.FormatBool(v), nil
	case "string":
		s, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("expected string")
		}
		return s, nil
	default:
		return "", fmt.Errorf("unknown type %s", def.Type)
	}
}

func (s *SettingsService) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]any, len(s.defs))
	for key, def := range s.defs {
		raw := s.cache[key]
		switch def.Type {
		case "float":
			v, _ := strconv.ParseFloat(raw, 64)
			result[key] = v
		case "int":
			v, _ := strconv.Atoi(raw)
			result[key] = v
		case "bool":
			v, _ := strconv.ParseBool(raw)
			result[key] = v
		case "string":
			result[key] = raw
		default:
			result[key] = raw
		}
	}
	return result
}

// Defaults returns all setting default values as typed values.
func (s *SettingsService) Defaults() map[string]any {
	result := make(map[string]any, len(s.defs))
	for _, def := range s.defs {
		switch def.Type {
		case "float":
			v, _ := strconv.ParseFloat(def.Default, 64)
			result[def.Key] = v
		case "int":
			v, _ := strconv.Atoi(def.Default)
			result[def.Key] = v
		case "bool":
			v, _ := strconv.ParseBool(def.Default)
			result[def.Key] = v
		case "string":
			result[def.Key] = def.Default
		default:
			result[def.Key] = def.Default
		}
	}
	return result
}

// Type conversion helpers for JSON values (which come as float64, bool, or string)

func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toInt(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(val)
	case float64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}
