package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrValidation is wrapped by every validation error so callers can map it
// to a client error (HTTP 400) instead of a server error.
var ErrValidation = errors.New("invalid configuration")

// MinAPITokenLength is the minimum length of a configured API token.
const MinAPITokenLength = 16

// Config holds the application configuration
type Config struct {
	KLF200  KLF200Config  `yaml:"klf200"`
	Server  ServerConfig  `yaml:"server"`
	Loxone  LoxoneConfig  `yaml:"loxone"`
	Logging LoggingConfig `yaml:"logging"`
}

// KLF200Config holds KLF-200 connection settings
type KLF200Config struct {
	Host              string        `yaml:"host"`
	Port              int           `yaml:"port"`
	Password          string        `yaml:"password"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
	RefreshInterval   time.Duration `yaml:"refresh_interval"`
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	APIToken     string        `yaml:"api_token"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "json" or "console"
}

// LoxoneConfig holds Loxone integration settings
type LoxoneConfig struct {
	UDPFeedback UDPFeedbackConfig `yaml:"udp_feedback" json:"udp_feedback"`
	Mappings    []NodeMapping     `yaml:"mappings" json:"mappings"`
}

// UDPFeedbackConfig holds UDP feedback settings
type UDPFeedbackConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	IP      string `yaml:"ip" json:"ip"`
	Port    int    `yaml:"port" json:"port"`
}

// NodeMapping maps a KLF-200 node to a Loxone virtual input
type NodeMapping struct {
	ID       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	NodeID   uint8  `yaml:"node_id" json:"node_id"`
	LoxoneID string `yaml:"loxone_id" json:"loxone_id"`
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		KLF200: KLF200Config{
			Host:              "",
			Port:              51200,
			Password:          "",
			ReconnectInterval: 30 * time.Second,
			RefreshInterval:   5 * time.Minute,
		},
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			APIToken:     "",
		},
		Loxone: LoxoneConfig{
			UDPFeedback: UDPFeedbackConfig{
				Enabled: false,
				IP:      "",
				Port:    7777,
			},
			Mappings: []NodeMapping{},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "console",
		},
	}
}

// Load loads configuration from a YAML file. A missing file is reported with
// an error that satisfies errors.Is(err, fs.ErrNotExist).
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ApplyEnv overrides KLF-200 host and password from the environment
// (KLF200_HOST, KLF200_PASSWORD) when set.
func (c *Config) ApplyEnv() {
	if host := os.Getenv("KLF200_HOST"); host != "" {
		c.KLF200.Host = host
	}
	if password := os.Getenv("KLF200_PASSWORD"); password != "" {
		c.KLF200.Password = password
	}
}

func validationError(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// IsValidationError reports whether err originates from configuration validation.
func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}

var validLogLevels = map[string]bool{
	"trace": true, "debug": true, "info": true, "warn": true, "error": true, "fatal": true, "panic": true,
}

// Validate validates the configuration (allows missing KLF200 credentials for initial setup)
func (c *Config) Validate() error {
	if c.KLF200.Port <= 0 || c.KLF200.Port > 65535 {
		return validationError("klf200.port must be between 1 and 65535")
	}
	if c.KLF200.ReconnectInterval <= 0 {
		return validationError("klf200.reconnect_interval must be positive")
	}
	if c.KLF200.RefreshInterval <= 0 {
		return validationError("klf200.refresh_interval must be positive")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return validationError("server.port must be between 1 and 65535")
	}
	// API token is optional - if not set, no authentication required
	if c.Server.APIToken != "" && len(c.Server.APIToken) < MinAPITokenLength {
		return validationError("server.api_token must be at least %d characters if set", MinAPITokenLength)
	}
	if c.Logging.Level != "" && !validLogLevels[c.Logging.Level] {
		return validationError("logging.level %q is not one of debug, info, warn, error", c.Logging.Level)
	}
	if c.Logging.Format != "" && c.Logging.Format != "json" && c.Logging.Format != "console" {
		return validationError("logging.format must be \"json\" or \"console\"")
	}
	return c.Loxone.Validate()
}

// Validate validates the Loxone integration settings.
func (l *LoxoneConfig) Validate() error {
	if l.UDPFeedback.Enabled {
		if l.UDPFeedback.IP == "" {
			return validationError("loxone.udp_feedback.ip is required when UDP feedback is enabled")
		}
		if l.UDPFeedback.Port <= 0 || l.UDPFeedback.Port > 65535 {
			return validationError("loxone.udp_feedback.port must be between 1 and 65535")
		}
	}

	seenID := make(map[string]bool, len(l.Mappings))
	seenLoxoneID := make(map[string]bool, len(l.Mappings))
	seenNodeID := make(map[uint8]string, len(l.Mappings))
	for i, m := range l.Mappings {
		if m.ID == "" {
			return validationError("loxone.mappings[%d]: id is required", i)
		}
		if seenID[m.ID] {
			return validationError("loxone.mappings: duplicate id %q", m.ID)
		}
		seenID[m.ID] = true

		if m.LoxoneID == "" {
			return validationError("loxone.mappings[%d]: loxone_id is required", i)
		}
		if seenLoxoneID[m.LoxoneID] {
			return validationError("loxone.mappings: loxone_id %q is used more than once", m.LoxoneID)
		}
		seenLoxoneID[m.LoxoneID] = true

		if m.Enabled {
			if other, ok := seenNodeID[m.NodeID]; ok {
				return validationError("loxone.mappings: node %d is mapped twice (%q and %q)", m.NodeID, other, m.LoxoneID)
			}
			seenNodeID[m.NodeID] = m.LoxoneID
		}
	}
	return nil
}

// IsKLF200Configured returns true if KLF200 host and password are set
func (c *Config) IsKLF200Configured() bool {
	return c.KLF200.Host != "" && c.KLF200.Password != ""
}

// writeFilePrivate writes data to path with owner-only permissions, since the
// configuration files contain the KLF-200 password and the API token.
func writeFilePrivate(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// WriteFile only applies the mode to newly created files.
	return os.Chmod(path, 0o600)
}

// Save saves the configuration to a YAML file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFilePrivate(path, data); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Clone returns a deep copy of the configuration. Callers can mutate the copy
// without affecting the shared config held by the ConfigManager.
func (c *Config) Clone() *Config {
	cp := *c
	if c.Loxone.Mappings != nil {
		cp.Loxone.Mappings = make([]NodeMapping, len(c.Loxone.Mappings))
		copy(cp.Loxone.Mappings, c.Loxone.Mappings)
	}
	return &cp
}

// LoadLoxoneConfig loads the UI-managed Loxone configuration (mappings and UDP
// feedback) from a sidecar file. This is kept separate from the main config so
// it survives Home Assistant add-on restarts, which regenerate config.yaml from
// the add-on options on every boot.
func LoadLoxoneConfig(path string) (*LoxoneConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lc LoxoneConfig
	if err := yaml.Unmarshal(data, &lc); err != nil {
		return nil, fmt.Errorf("failed to parse loxone config: %w", err)
	}
	if lc.Mappings == nil {
		lc.Mappings = []NodeMapping{}
	}
	if err := lc.Validate(); err != nil {
		return nil, err
	}

	return &lc, nil
}

// SaveLoxoneConfig persists the UI-managed Loxone configuration to its sidecar file.
func SaveLoxoneConfig(path string, lc *LoxoneConfig) error {
	data, err := yaml.Marshal(lc)
	if err != nil {
		return fmt.Errorf("failed to marshal loxone config: %w", err)
	}

	if err := writeFilePrivate(path, data); err != nil {
		return fmt.Errorf("failed to write loxone config: %w", err)
	}

	return nil
}
