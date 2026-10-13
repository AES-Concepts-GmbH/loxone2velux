package gateway

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200/klf200test"
)

const testPassword = "velux-secret"

func testNodes() []klf200test.Node {
	return []klf200test.Node{
		{ID: 0, Name: "Dachfenster", Type: klf200.NodeTypeWindowOpenerWithRainSensor, Position: 0},
		{ID: 1, Name: "Rollladen", Type: klf200.NodeTypeRollerShutter, Position: klf200.PositionMax},
	}
}

func klfConfig(srv *klf200test.Server, password string) *config.KLF200Config {
	return &config.KLF200Config{
		Host:              srv.Host(),
		Port:              srv.Port(),
		Password:          password,
		ReconnectInterval: 100 * time.Millisecond,
		RefreshInterval:   150 * time.Millisecond,
	}
}

func udpListener(t *testing.T) (int, <-chan string) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	msgs := make(chan string, 64)
	go func() {
		buf := make([]byte, 512)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			msgs <- string(buf[:n])
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port, msgs
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// collect drains msgs until a message containing want arrives or the timeout expires.
func collect(t *testing.T, msgs <-chan string, want string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m := <-msgs:
			if strings.Contains(m, want) {
				return
			}
		case <-deadline:
			t.Fatalf("UDP message %q not received", want)
		}
	}
}

func startService(t *testing.T, srv *klf200test.Server, loxoneCfg *config.LoxoneConfig) *Service {
	t.Helper()
	s := NewService(klfConfig(srv, testPassword), loxoneCfg, zerolog.Nop(), WithTLSConfig(srv.TLSConfig))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { s.Stop() })
	return s
}

func TestStartLoadsNodesAndCommands(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)

	if !s.IsConnected() || !s.IsConfigured() {
		t.Fatal("expected connected service")
	}
	if s.GetNodeCount() != 2 {
		t.Fatalf("node count = %d", s.GetNodeCount())
	}
	node, ok := s.GetNode(1)
	if !ok || node.Name != "Rollladen" || node.PositionPercent != 100 {
		t.Fatalf("node 1 = %+v ok=%v", node, ok)
	}
	if _, ok := s.GetNode(99); ok {
		t.Error("unknown node found")
	}
	if len(s.GetNodes()) != 2 {
		t.Error("GetNodes")
	}

	ctx := context.Background()
	if err := s.Open(ctx, 1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "node 1 open", func() bool { n, _ := s.GetNode(1); return n.PositionPercent == 0 })
	if err := s.SetPosition(ctx, 1, 50); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "node 1 at 50%", func() bool { n, _ := s.GetNode(1); return n.PositionPercent > 49 && n.PositionPercent < 51 })
	if err := s.Close(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.StopNode(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if len(srv.Commands()) != 4 {
		t.Errorf("server saw %d commands", len(srv.Commands()))
	}

	// Start is idempotent
	if err := s.Start(ctx); err != nil {
		t.Error(err)
	}
}

func TestUDPFeedbackAndSensors(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	port, msgs := udpListener(t)
	loxoneCfg := &config.LoxoneConfig{
		UDPFeedback: config.UDPFeedbackConfig{Enabled: true, IP: "127.0.0.1", Port: port},
		Mappings: []config.NodeMapping{
			{ID: "a", NodeID: 1, LoxoneID: "rollladen", Enabled: true},
			{ID: "b", NodeID: 0, LoxoneID: "dach", Enabled: false},
		},
	}
	s := startService(t, srv, loxoneCfg)
	if s.GetUDPSender() == nil || !s.GetUDPSender().IsEnabled() {
		t.Fatal("UDP sender not enabled")
	}
	if s.GetMappingManager().GetByNodeID(1) == nil {
		t.Fatal("mapping not loaded")
	}

	if err := s.SetPosition(context.Background(), 1, 25); err != nil {
		t.Fatal(err)
	}
	collect(t, msgs, "rollladen/position:25")
	collect(t, msgs, "rollladen/state:")

	// Unmapped (disabled) node sends nothing with its id
	if err := s.SetPosition(context.Background(), 0, 75); err != nil {
		t.Fatal(err)
	}
	// Sensor change: rain on node 0 -> rain:1 to the enabled mapping
	srv.SetLimitation(0, klf200.LimitationOriginatorRain)
	if err := s.RefreshSensorStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	collect(t, msgs, "rollladen/rain:1")
	if !s.GetSensorStatus().RainDetected {
		t.Error("rain not detected")
	}
	for {
		select {
		case m := <-msgs:
			if strings.HasPrefix(m, "dach/") {
				t.Fatalf("disabled mapping produced feedback: %s", m)
			}
			continue
		default:
		}
		break
	}
}

func TestNotConfiguredAndLateConfig(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	cfg := &config.KLF200Config{Host: "", Port: 1, Password: "", ReconnectInterval: 100 * time.Millisecond, RefreshInterval: time.Hour}
	s := NewService(cfg, nil, zerolog.Nop(), WithTLSConfig(srv.TLSConfig))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start without config must not fail: %v", err)
	}
	t.Cleanup(func() { s.Stop() })

	if s.IsConfigured() || s.IsConnected() {
		t.Fatal("unexpectedly configured/connected")
	}
	ctx := context.Background()
	if err := s.Open(ctx, 0); err != ErrNotConnected {
		t.Errorf("Open err = %v", err)
	}
	if err := s.Close(ctx, 0); err != ErrNotConnected {
		t.Errorf("Close err = %v", err)
	}
	if err := s.StopNode(ctx, 0); err != ErrNotConnected {
		t.Errorf("Stop err = %v", err)
	}
	if err := s.SetPosition(ctx, 0, 1); err != ErrNotConnected {
		t.Errorf("SetPosition err = %v", err)
	}
	if err := s.RefreshSensorStatus(ctx); err != ErrNotConnected {
		t.Errorf("RefreshSensorStatus err = %v", err)
	}
	if err := s.Reconnect(ctx); err == nil {
		t.Error("Reconnect without config should fail")
	}

	// Configure later: the reconnect loop picks it up and node updates work
	s.UpdateConfig(klfConfig(srv, testPassword))
	waitFor(t, "late connect", s.IsConnected)
	waitFor(t, "nodes loaded", func() bool { return s.GetNodeCount() == 2 })
	srv.PushPositionChange(0, klf200.PercentToPosition(60))
	waitFor(t, "pushed update applied", func() bool { n, _ := s.GetNode(0); return n.PositionPercent == 60 })
}

func TestReconnectAfterServerDrop(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)
	srv.DropConnections()
	waitFor(t, "disconnect noticed", func() bool { return !s.IsConnected() })
	waitFor(t, "automatic reconnect", s.IsConnected)
	if srv.Connections() < 2 {
		t.Errorf("connections = %d", srv.Connections())
	}

	// Manual reconnect
	if err := s.Reconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.IsConnected() {
		t.Error("not connected after manual reconnect")
	}
}

func TestReconnectWithWrongPassword(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)
	s.UpdateConfig(klfConfig(srv, "wrong"))
	if err := s.Reconnect(context.Background()); err == nil {
		t.Fatal("expected authentication error")
	}
	if s.IsConnected() {
		t.Error("should be disconnected after failed authentication")
	}
}

func TestKeepAliveDropsDeadConnection(t *testing.T) {
	oldInterval, oldTimeout := keepAliveInterval, keepAliveTimeout
	keepAliveInterval, keepAliveTimeout = 50*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { keepAliveInterval, keepAliveTimeout = oldInterval, oldTimeout })

	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)
	srv.SetSilent(true)
	waitFor(t, "dead connection dropped", func() bool { return srv.Connections() >= 2 })
	srv.SetSilent(false)
	waitFor(t, "reconnected", s.IsConnected)
}

func TestInitialConnectFailureThenRecover(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	cfg := klfConfig(srv, "wrong")
	s := NewService(cfg, nil, zerolog.Nop(), WithTLSConfig(srv.TLSConfig))
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("expected initial connection error")
	}
	t.Cleanup(func() { s.Stop() })
	s.UpdateConfig(klfConfig(srv, testPassword))
	waitFor(t, "recovered", s.IsConnected)
}

func TestStopIdempotent(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if s.IsConnected() {
		t.Error("still connected after Stop")
	}
}
