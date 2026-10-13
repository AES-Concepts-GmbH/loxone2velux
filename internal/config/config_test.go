package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLoadMissingFileIsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestLoadAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `
klf200:
  host: "10.0.0.5"
  password: "pw"
  reconnect_interval: 10s
server:
  port: 9000
  api_token: "0123456789abcdef"
logging:
  level: debug
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.KLF200.Host != "10.0.0.5" || cfg.KLF200.Port != 51200 || cfg.KLF200.ReconnectInterval != 10*time.Second {
		t.Errorf("klf200 = %+v", cfg.KLF200)
	}
	if cfg.KLF200.RefreshInterval != 5*time.Minute {
		t.Errorf("default refresh interval not applied: %v", cfg.KLF200.RefreshInterval)
	}
	if cfg.Server.Port != 9000 || !cfg.IsKLF200Configured() {
		t.Errorf("server = %+v configured=%v", cfg.Server, cfg.IsKLF200Configured())
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]func(c *Config){
		"klf port":       func(c *Config) { c.KLF200.Port = 70000 },
		"server port":    func(c *Config) { c.Server.Port = 0 },
		"short token":    func(c *Config) { c.Server.APIToken = "short" },
		"log level":      func(c *Config) { c.Logging.Level = "verbose" },
		"log format":     func(c *Config) { c.Logging.Format = "xml" },
		"zero reconnect": func(c *Config) { c.KLF200.ReconnectInterval = 0 },
		"udp without ip": func(c *Config) { c.Loxone.UDPFeedback.Enabled = true },
		"udp bad port":   func(c *Config) { c.Loxone.UDPFeedback = UDPFeedbackConfig{Enabled: true, IP: "1.2.3.4", Port: 0} },
		"mapping no id":  func(c *Config) { c.Loxone.Mappings = []NodeMapping{{LoxoneID: "a"}} },
		"mapping no lox": func(c *Config) { c.Loxone.Mappings = []NodeMapping{{ID: "1"}} },
		"dup mapping id": func(c *Config) { c.Loxone.Mappings = []NodeMapping{{ID: "1", LoxoneID: "a"}, {ID: "1", LoxoneID: "b"}} },
		"dup loxone id":  func(c *Config) { c.Loxone.Mappings = []NodeMapping{{ID: "1", LoxoneID: "a"}, {ID: "2", LoxoneID: "a"}} },
		"dup enabled node": func(c *Config) {
			c.Loxone.Mappings = []NodeMapping{{ID: "1", LoxoneID: "a", NodeID: 3, Enabled: true}, {ID: "2", LoxoneID: "b", NodeID: 3, Enabled: true}}
		},
	}
	for name, mutate := range cases {
		cfg := DefaultConfig()
		mutate(cfg)
		err := cfg.Validate()
		if err == nil {
			t.Errorf("%s: expected validation error", name)
			continue
		}
		if !IsValidationError(err) {
			t.Errorf("%s: error does not wrap ErrValidation: %v", name, err)
		}
	}

	// Same node mapped twice is fine when one mapping is disabled
	cfg := DefaultConfig()
	cfg.Loxone.Mappings = []NodeMapping{{ID: "1", LoxoneID: "a", NodeID: 3, Enabled: true}, {ID: "2", LoxoneID: "b", NodeID: 3, Enabled: false}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled duplicate node rejected: %v", err)
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("default config invalid: %v", err)
	}
}

func TestApplyEnv(t *testing.T) {
	t.Setenv("KLF200_HOST", "klf.local")
	t.Setenv("KLF200_PASSWORD", "secret")
	cfg := DefaultConfig()
	cfg.ApplyEnv()
	if cfg.KLF200.Host != "klf.local" || cfg.KLF200.Password != "secret" || !cfg.IsKLF200Configured() {
		t.Errorf("env not applied: %+v", cfg.KLF200)
	}
}

func TestCloneIsDeep(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Loxone.Mappings = []NodeMapping{{ID: "1", LoxoneID: "a"}}
	cp := cfg.Clone()
	cp.Loxone.Mappings[0].LoxoneID = "changed"
	cp.Loxone.Mappings = append(cp.Loxone.Mappings, NodeMapping{ID: "2", LoxoneID: "b"})
	if cfg.Loxone.Mappings[0].LoxoneID != "a" || len(cfg.Loxone.Mappings) != 1 {
		t.Error("Clone shares the mappings slice")
	}
}

func TestSaveAndLoxoneSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := DefaultConfig()
	cfg.KLF200.Password = "pw"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("config file mode = %o, want 600", info.Mode().Perm())
		}
	}

	lc := &LoxoneConfig{
		UDPFeedback: UDPFeedbackConfig{Enabled: true, IP: "1.2.3.4", Port: 7777},
		Mappings:    []NodeMapping{{ID: "1", Name: "n", NodeID: 2, LoxoneID: "x", Enabled: true}},
	}
	sidecar := filepath.Join(dir, "loxone.yaml")
	if err := SaveLoxoneConfig(sidecar, lc); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLoxoneConfig(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if got.UDPFeedback != lc.UDPFeedback || len(got.Mappings) != 1 || got.Mappings[0] != lc.Mappings[0] {
		t.Errorf("sidecar round trip: %+v", got)
	}

	// An invalid sidecar is rejected
	if err := os.WriteFile(sidecar, []byte("mappings:\n  - id: a\n    loxone_id: x\n  - id: a\n    loxone_id: y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLoxoneConfig(sidecar); !IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}
