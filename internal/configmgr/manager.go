// Package configmgr holds the runtime configuration, persists changes and
// applies them to the running gateway.
package configmgr

import (
	"context"
	"fmt"
	"reflect"
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
	managed          bool
}

// SetManagedConfig makes HA options authoritative for all non-Loxone settings.
// Call before exposing the manager to requests.
func (m *Manager) SetManagedConfig()   { m.managed = true }
func (m *Manager) ManagedConfig() bool { return m.managed }

// New creates a Manager. configPath is where the full configuration is saved,
// loxonePath (optional) is the sidecar file for the UI-managed Loxone settings.
func New(cfg *config.Config, configPath, loxonePath string, gw *gateway.Service, logger zerolog.Logger) *Manager {
	return &Manager{
		cfg:              cfg.Clone(),
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
	return m.updateConfig(cfg.Clone())
}

// ModifyConfig serializes the entire read/modify/validate/persist/apply operation.
func (m *Manager) ModifyConfig(change func(*config.Config) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg.Clone()
	if err := change(cfg); err != nil {
		return err
	}
	return m.updateConfig(cfg)
}

func (m *Manager) updateConfig(cfg *config.Config) error {
	if m.managed {
		before, after := m.cfg.Clone(), cfg.Clone()
		before.Loxone, after.Loxone = config.LoxoneConfig{}, config.LoxoneConfig{}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("%w: change connection, token and logging in Home Assistant add-on options", config.ErrValidation)
		}
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return err
	}
	commitUDP := func() {}
	if m.gateway != nil && m.cfg.Loxone.UDPFeedback != cfg.Loxone.UDPFeedback {
		commit, abort, err := m.gateway.GetUDPSender().Prepare(cfg.Loxone.UDPFeedback)
		if err != nil {
			return fmt.Errorf("prepare UDP feedback: %w", err)
		}
		defer abort()
		commitUDP = commit
	}

	// Exactly one authoritative file per mode avoids partially committed pairs.
	var err error
	if m.managed {
		err = config.SaveLoxoneConfig(m.loxonePath, &cfg.Loxone)
	} else {
		err = cfg.Save(m.configPath)
	}
	if err != nil {
		return fmt.Errorf("persist configuration: %w", err)
	}
	commitUDP()

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
		if mappingMgr := m.gateway.GetMappingManager(); mappingMgr != nil {
			mappingMgr.Load(cfg.Loxone.Mappings)
		}
	}

	m.cfg = cfg
	return nil
}
