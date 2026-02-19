package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/intelsk/backend/config"
	"github.com/intelsk/backend/services"
)

var rootDir string

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "extract":
		runExtract(os.Args[2:])
	case "process":
		runProcess(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: backend <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  extract   Extract frames from a single video file")
	fmt.Fprintln(os.Stderr, "  process   Extract frames from all videos for a camera+date")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Common flags:")
	fmt.Fprintln(os.Stderr, "  -root     Project root directory (default: parent of backend/)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'backend <command> -help' for details.")
}

func addRootFlag(fs *flag.FlagSet) {
	fs.StringVar(&rootDir, "root", "", "project root directory (default: parent of backend/)")
}

func resolveRoot() string {
	if rootDir != "" {
		abs, err := filepath.Abs(rootDir)
		if err != nil {
			log.Fatalf("resolving root: %v", err)
		}
		return abs
	}

	// Default: parent of the directory containing the executable's go.mod,
	// which in practice means ../  when running from backend/
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Dir(filepath.Dir(exe))
		if _, err := os.Stat(filepath.Join(candidate, "config")); err == nil {
			return candidate
		}
	}

	// Fallback: assume cwd is backend/, go up one level
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("getting cwd: %v", err)
	}
	candidate := filepath.Dir(cwd)
	if _, err := os.Stat(filepath.Join(candidate, "config")); err == nil {
		return candidate
	}

	// Last resort: use cwd itself (user may already be at project root)
	return cwd
}

func loadAppConfig() *config.AppConfig {
	root := resolveRoot()
	appYaml := filepath.Join(root, "config", "app.yaml")
	extractionYaml := filepath.Join(root, "config", "extraction.yaml")

	cfg, err := config.LoadConfig(appYaml, extractionYaml)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	// Make relative paths absolute against project root
	if !filepath.IsAbs(cfg.App.DataDir) {
		cfg.App.DataDir = filepath.Join(root, cfg.App.DataDir)
	}
	if !filepath.IsAbs(cfg.Extraction.StoragePath) {
		cfg.Extraction.StoragePath = filepath.Join(root, cfg.Extraction.StoragePath)
	}

	return cfg
}

func runExtract(args []string) {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	video := fs.String("video", "", "path to video file (required)")
	addRootFlag(fs)
	fs.Parse(args)

	if *video == "" {
		fmt.Fprintln(os.Stderr, "error: -video flag is required")
		fs.Usage()
		os.Exit(1)
	}

	cfg := loadAppConfig()
	extractVideo(cfg, *video)
}

func runProcess(args []string) {
	fs := flag.NewFlagSet("process", flag.ExitOnError)
	camera := fs.String("camera", "", "camera ID (required)")
	date := fs.String("date", "", "date in YYYY-MM-DD format (required)")
	addRootFlag(fs)
	fs.Parse(args)

	if *camera == "" || *date == "" {
		fmt.Fprintln(os.Stderr, "error: -camera and -date flags are required")
		fs.Usage()
		os.Exit(1)
	}

	cfg := loadAppConfig()

	videosDir := filepath.Join(cfg.App.DataDir, "videos", *camera, *date)
	entries, err := os.ReadDir(videosDir)
	if err != nil {
		log.Fatalf("reading videos directory %s: %v", videosDir, err)
	}

	var videos []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") {
			videos = append(videos, filepath.Join(videosDir, e.Name()))
		}
	}

	if len(videos) == 0 {
		log.Fatalf("no MP4 files found in %s", videosDir)
	}

	fmt.Printf("Found %d video(s) in %s\n", len(videos), videosDir)
	for _, v := range videos {
		extractVideo(cfg, v)
	}
}

func extractVideo(cfg *config.AppConfig, videoPath string) {
	fmt.Printf("Extracting frames from %s (interval=%ds, quality=%d)\n",
		videoPath, cfg.Extraction.TimeIntervalSec, cfg.Extraction.OutputQuality)

	// Derive output directory from video path structure:
	// data/videos/{camera}/{date}/HHMM.mp4 → data/frames/{camera}/{date}/
	abs, err := filepath.Abs(videoPath)
	if err != nil {
		log.Fatalf("resolving path: %v", err)
	}
	parts := strings.Split(filepath.ToSlash(abs), "/")
	if len(parts) < 3 {
		log.Fatalf("cannot parse camera/date from path: %s", videoPath)
	}
	dateStr := parts[len(parts)-2]
	cameraID := parts[len(parts)-3]

	outputDir := filepath.Join(cfg.Extraction.StoragePath, cameraID, dateStr)

	frames, err := services.ExtractFramesTime(
		videoPath,
		outputDir,
		cfg.Extraction.TimeIntervalSec,
		cfg.Extraction.OutputQuality,
	)
	if err != nil {
		log.Fatalf("extraction failed: %v", err)
	}
	fmt.Printf("Extracted %d frame(s)\n", len(frames))

	if cfg.Extraction.DedupEnabled {
		fmt.Printf("Running de-duplication (threshold=%d)...\n", cfg.Extraction.DedupPHashThreshold)
		before := len(frames)
		frames, err = services.DeduplicateFrames(frames, cfg.Extraction.DedupPHashThreshold)
		if err != nil {
			log.Fatalf("dedup failed: %v", err)
		}
		fmt.Printf("De-duplication: %d → %d frames (%d duplicates removed)\n",
			before, len(frames), before-len(frames))
	}

	if err := services.WriteManifest(outputDir, frames); err != nil {
		log.Fatalf("writing manifest: %v", err)
	}
	fmt.Printf("Manifest written to %s/manifest.json\n", outputDir)
}
