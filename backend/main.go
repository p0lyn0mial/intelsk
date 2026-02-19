package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/intelsk/backend/cmd/server"
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
	case "index":
		runIndex(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "  index     Index extracted frames via CLIP embeddings")
	fmt.Fprintln(os.Stderr, "  search    Search indexed frames by text query")
	fmt.Fprintln(os.Stderr, "  serve     Start the HTTP API server")
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
	if !filepath.IsAbs(cfg.Storage.DBPath) {
		cfg.Storage.DBPath = filepath.Join(root, cfg.Storage.DBPath)
	}
	if !filepath.IsAbs(cfg.Process.HistoryPath) {
		cfg.Process.HistoryPath = filepath.Join(root, cfg.Process.HistoryPath)
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

func runIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
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

	// Resolve frames directory
	framesDir, err := services.ResolveFramesDir(cfg.Extraction.StoragePath, *camera, *date)
	if err != nil {
		log.Fatalf("resolving frames directory: %v", err)
	}

	// Init ML client and wait for sidecar
	mlClient := services.NewMLClient(cfg.MLService.URL)
	fmt.Printf("Waiting for ML sidecar at %s...\n", cfg.MLService.URL)
	if err := mlClient.WaitForReady(60 * time.Second); err != nil {
		log.Fatalf("ML sidecar not ready: %v", err)
	}
	fmt.Println("ML sidecar ready")

	// Init storage
	storage, err := services.NewStorage(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("opening storage: %v", err)
	}
	defer storage.Close()

	// Build and run pipeline
	pipeline := services.NewPipeline(mlClient, storage, cfg.CLIP.BatchSize)
	progress := make(chan services.ProgressEvent, 10)

	go func() {
		for ev := range progress {
			if ev.FramesTotal > 0 {
				fmt.Printf("[%s] %s — %d/%d frames\n",
					ev.Stage, ev.Message, ev.FramesDone, ev.FramesTotal)
			} else {
				fmt.Printf("[%s] %s\n", ev.Stage, ev.Message)
			}
		}
	}()

	if err := pipeline.IndexFrames(framesDir, progress); err != nil {
		log.Fatalf("indexing failed: %v", err)
	}
	close(progress)

	fmt.Println("Indexing complete")
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	text := fs.String("text", "", "text query (required)")
	camera := fs.String("camera", "", "camera ID filter (optional)")
	limit := fs.Int("limit", 20, "max results")
	addRootFlag(fs)
	fs.Parse(args)

	if *text == "" {
		fmt.Fprintln(os.Stderr, "error: -text flag is required")
		fs.Usage()
		os.Exit(1)
	}

	cfg := loadAppConfig()

	mlClient := services.NewMLClient(cfg.MLService.URL)
	if err := mlClient.HealthCheck(); err != nil {
		log.Fatalf("ML sidecar health check failed: %v", err)
	}

	var cameraIDs []string
	if *camera != "" {
		cameraIDs = []string{*camera}
	}

	results, err := mlClient.SearchByText(cfg.Storage.DBPath, *text, cameraIDs, "", "", *limit)
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}

	fmt.Printf("Results for query: %q\n\n", *text)
	fmt.Print(services.FormatResultsTable(results))
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addRootFlag(fs)
	fs.Parse(args)

	cfg := loadAppConfig()
	server.Start(cfg)
}
