package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/api"
	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/configmgr"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
)

var version = "dev"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	managed := flag.Bool("managed-config", false, "Home Assistant options manage non-Loxone settings")
	flag.Parse()

	if *showVersion {
		fmt.Println("loxone2velux", version)
		os.Exit(0)
	}

	// Load configuration. A missing file is not an error: defaults plus the
	// KLF200_HOST / KLF200_PASSWORD environment variables are used instead.
	cfg, err := config.Load(*configPath)
	configFromFile := true
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to load configuration: %s\n", err.Error())
			os.Exit(1)
		}
		configFromFile = false
		cfg = config.DefaultConfig()
		cfg.ApplyEnv()
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", err.Error())
			os.Exit(1)
		}
	}

	// Setup logger
	logger := setupLogger(cfg.Logging)
	if !configFromFile {
		logger.Warn().Str("path", *configPath).Msg("Config file not found, using defaults and environment variables")
	}

	// Overlay the UI-managed Loxone settings from the sidecar file, if present.
	// This is kept separate from config.yaml so mappings and UDP feedback
	// survive Home Assistant add-on restarts that regenerate config.yaml.
	loxonePath := filepath.Join(filepath.Dir(*configPath), "loxone.yaml")
	if *managed {
		if lc, lerr := config.LoadLoxoneConfig(loxonePath); lerr == nil {
			cfg.Loxone = *lc
			logger.Info().Str("path", loxonePath).Msg("Loaded Loxone configuration from sidecar")
		} else if !errors.Is(lerr, fs.ErrNotExist) {
			logger.Error().Err(lerr).Msg("Failed to load Loxone sidecar config")
			os.Exit(1)
		}
	} else if err := config.MigrateLoxoneSidecar(*configPath, loxonePath, cfg); err != nil {
		logger.Error().Err(err).Msg("Failed to migrate legacy Loxone configuration")
		os.Exit(1)
	}

	logger.Info().
		Str("version", version).
		Str("klf200_host", cfg.KLF200.Host).
		Int("klf200_port", cfg.KLF200.Port).
		Int("server_port", cfg.Server.Port).
		Msg("Starting Loxone2Velux Gateway")

	if cfg.Server.APIToken == "" {
		logger.Warn().Msg("No API token configured - anyone who can reach the HTTP port can control the devices. Set server.api_token to enable authentication.")
	}

	// Create gateway service. Background loops are always started so that a
	// KLF-200 configured later via the API is picked up.
	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, logger)
	if err := gw.Start(context.Background()); err != nil {
		// Don't fail - will retry in background
		logger.Warn().Err(err).Msg("Initial KLF-200 connection failed, will retry in background")
	}

	// Create config manager
	configMgr := configmgr.New(cfg, *configPath, loxonePath, gw, logger)
	if *managed {
		configMgr.SetManagedConfig()
	}

	// Create and start API server
	server := api.NewServer(&cfg.Server, gw, logger, configMgr, version)

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Start() }()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	serverFailed := false
	select {
	case <-quit:
	case err := <-serverErrors:
		logger.Error().Err(err).Msg("HTTP server stopped")
		serverFailed = err != nil
	}
	signal.Stop(quit)

	logger.Info().Msg("Shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Server shutdown error")
	}

	if err := gw.Stop(); err != nil {
		logger.Error().Err(err).Msg("Gateway shutdown error")
	}

	logger.Info().Msg("Goodbye!")
	if serverFailed {
		os.Exit(1)
	}
}

func setupLogger(cfg config.LoggingConfig) zerolog.Logger {
	configmgr.ApplyLogLevel(cfg.Level)

	// Set output format
	var logger zerolog.Logger
	if cfg.Format == "json" {
		logger = zerolog.New(os.Stdout)
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	}

	return logger.With().Timestamp().Caller().Logger()
}
