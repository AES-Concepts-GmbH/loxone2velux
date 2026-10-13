// Package configmgr holds the runtime configuration, persists changes and
// applies them to the running gateway.
package configmgr

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
)

// Manager manages configuration with persistence.
type Manager struct {
	cfg        *config.Config
	configPath string
	loxonePath string
	gateway    *gateway.Service
	mu         sync.RWMutex
	logger     zerolog.Logger

	// setLogLevel applies a new log level process-wide (zerolog global level).
	setLogLevel func(level string)
	// reconnectTimeout bounds the background reconnect after a KLF-200 change.
	reconnectTimeout time.Duration
}

// New creates a Manager. configPath is where the full configuration is saved,
// loxonePath (optional) is the sidecar file for the UI-managed Loxone settings.
func New(cfg *config.Config, configPath, loxonePath string, gw *gateway.Service, logger zerolog.Logger) *Manager {
	return &Manager{
		cfg:              cfg,
		configPath:       configPath,
		loxonePath:       loxonePath,
		gateway:          gw,
		logger:           logger,
		setLogLevel:      ApplyLogLevel,
		reconnectTimeout: 30 * time.Second,
	}
}

// ApplyLogLevel sets zerolog's global level from a level name; unknown or
// empty names fall back to info.
func ApplyLogLevel(levelStr string) {
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil || levelStr == "" {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
}

// GetConfig returns a copy of the current configuration. A copy is returned so
// callers (HTTP handlers) can mutate it freely without racing on the shared config.
func (m *Manager) GetConfig() *config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Clone()
}

// GetAPIToken returns the currently active API token ("" = no auth).
func (m *Manager) GetAPIToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Server.APIToken
}

// GetConfigPath returns the path to the config file.
func (m *Manager) GetConfigPath() string {
	return m.configPath
}

// UpdateConfig validates, persists and applies a new configuration.
func (m *Manager) UpdateConfig(cfg *config.Config) error {
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
		m.setLogLevel(cfg.Logging.Level)
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
	if m.gateway != nil && m.cfg.KLF200 != cfg.KLF200 {
		m.gateway.UpdateConfig(&cfg.KLF200)
		connChanged := m.cfg.KLF200.Host != cfg.KLF200.Host ||
			m.cfg.KLF200.Port != cfg.KLF200.Port ||
			m.cfg.KLF200.Password != cfg.KLF200.Password
		if connChanged {
			// Reconnect in the background so the new credentials take effect without
			// blocking the HTTP response.
			gw, timeout := m.gateway, m.reconnectTimeout
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				if err := gw.Reconnect(ctx); err != nil {
					m.logger.Warn().Err(err).Msg("Reconnect after config change failed")
				}
			}()
		}
	}

	// Reconfigure UDP sender and mappings
	if m.gateway != nil {
		if udpSender := m.gateway.GetUDPSender(); udpSender != nil {
			if err := udpSender.Configure(cfg.Loxone.UDPFeedback); err != nil {
				m.logger.Error().Err(err).Msg("Failed to reconfigure UDP sender")
			}
		}
		if mappingMgr := m.gateway.GetMappingManager(); mappingMgr != nil {
			mappingMgr.Load(cfg.Loxone.Mappings)
		}
	}

	m.cfg = cfg
	return nil
}
