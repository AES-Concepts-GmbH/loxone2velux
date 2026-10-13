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
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/api"
	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
)

var version = "dev"

// ConfigManager manages configuration with persistence
type ConfigManager struct {
	cfg        *config.Config
	configPath string
	loxonePath string
	gateway    *gateway.Service
	mu         sync.RWMutex
	logger     zerolog.Logger
}

// NewConfigManager creates a new ConfigManager
func NewConfigManager(cfg *config.Config, configPath, loxonePath string, gw *gateway.Service, logger zerolog.Logger) *ConfigManager {
	return &ConfigManager{
		cfg:        cfg,
		configPath: configPath,
		loxonePath: loxonePath,
		gateway:    gw,
		logger:     logger,
	}
}

// GetConfig returns a copy of the current configuration. A copy is returned so
// callers (HTTP handlers) can mutate it freely without racing on the shared config.
func (m *ConfigManager) GetConfig() *config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Clone()
}

// GetAPIToken returns the currently active API token ("" = no auth).
func (m *ConfigManager) GetAPIToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Server.APIToken
}

// GetConfigPath returns the path to the config file
func (m *ConfigManager) GetConfigPath() string {
	return m.configPath
}

// UpdateConfig validates, persists and applies a new configuration.
func (m *ConfigManager) UpdateConfig(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Save to file
	if err := cfg.Save(m.configPath); err != nil {
		m.logger.Error().Err(err).Msg("Failed to save config file")
		// Continue anyway - config is updated in memory
	} else {
		m.logger.Info().Str("path", m.configPath).Msg("Configuration saved")
	}

	// Persist the UI-managed Loxone settings to the sidecar file so they
	// survive add-on restarts (which regenerate config.yaml from add-on options).
	if m.loxonePath != "" {
		if err := config.SaveLoxoneConfig(m.loxonePath, &cfg.Loxone); err != nil {
			m.logger.Error().Err(err).Msg("Failed to save Loxone sidecar config")
		} else {
			m.logger.Info().Str("path", m.loxonePath).Msg("Loxone configuration saved")
		}
	}

	// Apply log level immediately
	if cfg.Logging.Level != m.cfg.Logging.Level {
		applyLogLevel(cfg.Logging.Level)
		m.logger.Info().Str("level", cfg.Logging.Level).Msg("Log level changed")
	}

	if cfg.Server.APIToken != m.cfg.Server.APIToken {
		if cfg.Server.APIToken == "" {
			m.logger.Warn().Msg("API token removed - the API is now reachable without authentication")
		} else {
			m.logger.Info().Msg("API token changed")
		}
	}

	// Update gateway config and reconnect if KLF-200 settings changed
	klfChanged := m.cfg.KLF200 != cfg.KLF200
	if klfChanged {
		m.gateway.UpdateConfig(&cfg.KLF200)
		connChanged := m.cfg.KLF200.Host != cfg.KLF200.Host ||
			m.cfg.KLF200.Port != cfg.KLF200.Port ||
			m.cfg.KLF200.Password != cfg.KLF200.Password
		if connChanged {
			// Reconnect in the background so the new credentials take effect without
			// blocking the HTTP response.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := m.gateway.Reconnect(ctx); err != nil {
					m.logger.Warn().Err(err).Msg("Reconnect after config change failed")
				}
			}()
		}
	}

	// Reconfigure UDP sender and mappings
	if udpSender := m.gateway.GetUDPSender(); udpSender != nil {
		if err := udpSender.Configure(cfg.Loxone.UDPFeedback); err != nil {
			m.logger.Error().Err(err).Msg("Failed to reconfigure UDP sender")
		}
	}
	if mappingMgr := m.gateway.GetMappingManager(); mappingMgr != nil {
		mappingMgr.Load(cfg.Loxone.Mappings)
	}

	m.cfg = cfg
	return nil
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version and exit")
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
	if lc, lerr := config.LoadLoxoneConfig(loxonePath); lerr == nil {
		cfg.Loxone = *lc
		logger.Info().Str("path", loxonePath).Msg("Loaded Loxone configuration from sidecar")
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		logger.Warn().Err(lerr).Str("path", loxonePath).Msg("Failed to load Loxone sidecar config")
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
	configMgr := NewConfigManager(cfg, *configPath, loxonePath, gw, logger)

	// Create and start API server
	server := api.NewServer(&cfg.Server, gw, logger, configMgr, version)

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			logger.Fatal().Err(err).Msg("Server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

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
}

func applyLogLevel(levelStr string) {
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil || levelStr == "" {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
}

func setupLogger(cfg config.LoggingConfig) zerolog.Logger {
	applyLogLevel(cfg.Level)

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
