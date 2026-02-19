package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AppSettings struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DataDir  string `yaml:"data_dir"`
	LogLevel string `yaml:"log_level"`
}

type ExtractionSettings struct {
	Method             string  `yaml:"method"`
	TimeIntervalSec    int     `yaml:"time_interval_sec"`
	MotionThreshold    float64 `yaml:"motion_threshold"`
	MinGapSec          float64 `yaml:"min_gap_sec"`
	OutputFormat       string  `yaml:"output_format"`
	OutputQuality      int     `yaml:"output_quality"`
	DedupEnabled       bool    `yaml:"dedup_enabled"`
	DedupPHashThreshold int    `yaml:"dedup_phash_threshold"`
	StoragePath        string  `yaml:"storage_path"`
}

type MLServiceSettings struct {
	URL string `yaml:"url"`
}

type StorageSettings struct {
	DBPath string `yaml:"db_path"`
}

type CLIPSettings struct {
	BatchSize int `yaml:"batch_size"`
}

type ProcessSettings struct {
	HistoryPath string `yaml:"history_path"`
}

type AppConfig struct {
	App        AppSettings        `yaml:"app"`
	Extraction ExtractionSettings `yaml:"extraction"`
	MLService  MLServiceSettings  `yaml:"mlservice"`
	Storage    StorageSettings    `yaml:"storage"`
	CLIP       CLIPSettings       `yaml:"clip"`
	Process    ProcessSettings    `yaml:"process"`
}

// LoadConfig reads and parses two YAML files (app config and extraction config)
// and merges them into a single AppConfig struct.
func LoadConfig(appYaml, extractionYaml string) (*AppConfig, error) {
	cfg := &AppConfig{}

	if err := loadYAML(appYaml, cfg); err != nil {
		return nil, fmt.Errorf("loading %s: %w", appYaml, err)
	}

	if err := loadYAML(extractionYaml, cfg); err != nil {
		return nil, fmt.Errorf("loading %s: %w", extractionYaml, err)
	}

	if cfg.App.Host == "" {
		cfg.App.Host = "0.0.0.0"
	}
	if cfg.App.Port == 0 {
		cfg.App.Port = 8000
	}
	if cfg.App.DataDir == "" {
		cfg.App.DataDir = "data"
	}
	if cfg.App.LogLevel == "" {
		cfg.App.LogLevel = "info"
	}
	if cfg.Extraction.TimeIntervalSec == 0 {
		cfg.Extraction.TimeIntervalSec = 5
	}
	if cfg.Extraction.OutputQuality == 0 {
		cfg.Extraction.OutputQuality = 85
	}
	if cfg.Extraction.StoragePath == "" {
		cfg.Extraction.StoragePath = "data/frames"
	}
	if cfg.MLService.URL == "" {
		cfg.MLService.URL = "http://localhost:8001"
	}
	if cfg.Storage.DBPath == "" {
		cfg.Storage.DBPath = "data/intelsk.db"
	}
	if cfg.CLIP.BatchSize == 0 {
		cfg.CLIP.BatchSize = 32
	}
	if cfg.Process.HistoryPath == "" {
		cfg.Process.HistoryPath = "data/process_history.json"
	}

	return cfg, nil
}

func loadYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
