package configmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200/klf200test"
)

func newManager(t *testing.T, gw *gateway.Service) (*Manager, string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	loxPath := filepath.Join(dir, "loxone.yaml")
	cfg := config.DefaultConfig()
	m := New(cfg, cfgPath, loxPath, gw, zerolog.Nop())
	return m, cfgPath, loxPath
}

func TestGettersAndClone(t *testing.T) {
	m, cfgPath, _ := newManager(t, nil)
	if m.GetConfigPath() != cfgPath {
		t.Error("GetConfigPath")
	}
	if m.GetAPIToken() != "" {
		t.Error("token should be empty")
	}
	c := m.GetConfig()
	c.Server.APIToken = "0123456789abcdef"
	if m.GetAPIToken() != "" {
		t.Error("GetConfig must return a copy")
	}
}

func TestUpdateConfigPersistsAndApplies(t *testing.T) {
	m, cfgPath, loxPath := newManager(t, nil)
	var appliedLevel string
	m.setLogLevel = func(l string) { appliedLevel = l }

	cfg := m.GetConfig()
	cfg.Server.APIToken = "0123456789abcdef"
	cfg.Logging.Level = "debug"
	cfg.Loxone.Mappings = []config.NodeMapping{{ID: "m1", NodeID: 1, LoxoneID: "x", Enabled: true}}
	if err := m.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	if m.GetAPIToken() != "0123456789abcdef" {
		t.Error("token not applied")
	}
	if appliedLevel != "debug" {
		t.Errorf("log level applied = %q", appliedLevel)
	}
	saved, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Server.APIToken != "0123456789abcdef" || saved.Logging.Level != "debug" {
		t.Errorf("saved config = %+v", saved)
	}
	side, err := config.LoadLoxoneConfig(loxPath)
	if err != nil || len(side.Mappings) != 1 || side.Mappings[0].ID != "m1" {
		t.Errorf("sidecar = %+v err=%v", side, err)
	}

	// Same level again does not re-apply
	appliedLevel = ""
	cfg = m.GetConfig()
	cfg.Server.APIToken = ""
	if err := m.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if appliedLevel != "" {
		t.Error("log level re-applied without change")
	}
	if m.GetAPIToken() != "" {
		t.Error("token removal not applied")
	}
}

func TestUpdateConfigValidationFailureLeavesStateUntouched(t *testing.T) {
	m, cfgPath, _ := newManager(t, nil)
	cfg := m.GetConfig()
	cfg.Server.APIToken = "short"
	err := m.UpdateConfig(cfg)
	if !config.IsValidationError(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
		t.Error("config file written despite validation failure")
	}
	if m.GetAPIToken() != "" {
		t.Error("invalid token applied")
	}
}

func TestUpdateConfigSurvivesUnwritablePath(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg, filepath.Join(t.TempDir(), "missing-dir", "config.yaml"), "", nil, zerolog.Nop())
	c := m.GetConfig()
	c.Logging.Level = "warn"
	if err := m.UpdateConfig(c); err != nil {
		t.Fatalf("UpdateConfig should keep the in-memory config: %v", err)
	}
	if m.GetConfig().Logging.Level != "warn" {
		t.Error("in-memory config not updated")
	}
}

func TestUpdateConfigReconfiguresGateway(t *testing.T) {
	srv := klf200test.NewServer(t, "pw",
		klf200test.Node{ID: 0, Name: "n0"},
	)
	klfCfg := config.KLF200Config{Host: "", Port: 1, Password: "", ReconnectInterval: time.Hour, RefreshInterval: time.Hour}
	gw := gateway.NewService(&klfCfg, &config.LoxoneConfig{}, zerolog.Nop(), gateway.WithTLSConfig(srv.TLSConfig))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gw.Stop() })

	m, _, _ := newManager(t, gw)
	m.reconnectTimeout = 5 * time.Second

	cfg := m.GetConfig()
	cfg.KLF200 = config.KLF200Config{Host: srv.Host(), Port: srv.Port(), Password: "pw", ReconnectInterval: time.Hour, RefreshInterval: time.Hour}
	cfg.Loxone.Mappings = []config.NodeMapping{{ID: "m", NodeID: 0, LoxoneID: "l", Enabled: true}}
	cfg.Loxone.UDPFeedback = config.UDPFeedbackConfig{Enabled: true, IP: "127.0.0.1", Port: 7777}
	if err := m.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !gw.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !gw.IsConnected() {
		t.Fatal("gateway did not reconnect after config change")
	}
	if gw.GetMappingManager().GetByNodeID(0) == nil {
		t.Error("mappings not loaded into gateway")
	}
	if !gw.GetUDPSender().IsEnabled() {
		t.Error("UDP sender not configured")
	}

	// Interval-only change must not reconnect
	before := srv.Connections()
	cfg = m.GetConfig()
	cfg.KLF200.RefreshInterval = 2 * time.Hour
	if err := m.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if srv.Connections() != before {
		t.Error("interval change triggered a reconnect")
	}
}

func TestApplyLogLevel(t *testing.T) {
	t.Cleanup(func() { zerolog.SetGlobalLevel(zerolog.InfoLevel) })
	ApplyLogLevel("debug")
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Error("debug not applied")
	}
	ApplyLogLevel("nonsense")
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Error("unknown level should fall back to info")
	}
	ApplyLogLevel("")
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Error("empty level should fall back to info")
	}
}
