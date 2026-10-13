package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200/klf200test"
)

// newConnectedServer starts a fake KLF-200 with two nodes, a connected
// gateway and the HTTP API on top of it.
func newConnectedServer(t *testing.T) (*httptest.Server, *klf200test.Server, *gateway.Service, *memConfigManager) {
	t.Helper()
	srv := klf200test.NewServer(t, "pw",
		klf200test.Node{ID: 0, Name: "Dachfenster", Type: klf200.NodeTypeWindowOpenerWithRainSensor, Position: 0},
		klf200test.Node{ID: 1, Name: "Rollladen", Type: klf200.NodeTypeRollerShutter, Position: klf200.PositionMax},
	)
	cfg := config.DefaultConfig()
	cfg.KLF200 = config.KLF200Config{Host: srv.Host(), Port: srv.Port(), Password: "pw", ReconnectInterval: time.Hour, RefreshInterval: time.Hour}
	mgr := &memConfigManager{cfg: cfg}

	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop(), gateway.WithTLSConfig(srv.TLSConfig))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatalf("gateway start: %v", err)
	}
	t.Cleanup(func() { gw.Stop() })
	mgr.gateway = gw

	s := NewServer(&cfg.Server, gw, zerolog.Nop(), mgr, "test")
	ts := httptest.NewServer(s.setupRoutes())
	t.Cleanup(ts.Close)
	return ts, srv, gw, mgr
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

func TestHealthAndNodesConnected(t *testing.T) {
	ts, _, _, _ := newConnectedServer(t)

	_, body := do(t, "GET", ts.URL+"/health", "", nil)
	var h HealthResponse
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "ok" || !h.Connected || h.NodeCount != 2 || h.Version != "test" {
		t.Errorf("health = %+v", h)
	}

	_, body = do(t, "GET", ts.URL+"/api/nodes", "", nil)
	var nodes NodesResponse
	if err := json.Unmarshal([]byte(body), &nodes); err != nil {
		t.Fatal(err)
	}
	if nodes.Count != 2 || len(nodes.Nodes) != 2 {
		t.Errorf("nodes = %+v", nodes)
	}

	resp, body := do(t, "GET", ts.URL+"/api/nodes/1", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"name":"Rollladen"`) {
		t.Errorf("node 1: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/nodes/7", "", nil); resp.StatusCode != 404 {
		t.Errorf("unknown node status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/nodes/abc", "", nil); resp.StatusCode != 400 {
		t.Errorf("bad node id status = %d", resp.StatusCode)
	}
}

func TestNodeCommandsConnected(t *testing.T) {
	ts, srv, gw, _ := newConnectedServer(t)

	cases := []struct {
		method, path, body string
		wantParam          uint16
	}{
		{"POST", "/api/nodes/1/position", `{"position":40}`, klf200.PercentToPosition(40)},
		{"POST", "/api/nodes/1/open", "", klf200.PositionMin},
		{"POST", "/api/nodes/1/close", "", klf200.PositionMax},
		{"POST", "/api/nodes/1/stop", "", klf200.PositionCurrent},
		{"GET", "/loxone/node/1/set/70", "", klf200.PercentToPosition(70)},
		{"GET", "/loxone/node/1/open", "", klf200.PositionMin},
		{"GET", "/loxone/node/1/close", "", klf200.PositionMax},
		{"GET", "/loxone/node/1/stop", "", klf200.PositionCurrent},
	}
	for i, c := range cases {
		resp, body := do(t, c.method, ts.URL+c.path, c.body, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s: status %d body %s", c.method, c.path, resp.StatusCode, body)
		}
		if c.method == "GET" && body != "OK" {
			t.Errorf("%s: body %q", c.path, body)
		}
		if c.method == "POST" && !strings.Contains(body, `"success":true`) {
			t.Errorf("%s: body %s", c.path, body)
		}
		cmds := srv.Commands()
		if len(cmds) != i+1 {
			t.Fatalf("%s: server has %d commands, want %d", c.path, len(cmds), i+1)
		}
		if cmds[i].MainParameter != c.wantParam || cmds[i].NodeIDs[0] != 1 {
			t.Errorf("%s: command = %+v, want param 0x%04x", c.path, cmds[i], c.wantParam)
		}
	}

	// Invalid bodies / values
	if resp, _ := do(t, "POST", ts.URL+"/api/nodes/1/position", `{"position":101}`, nil); resp.StatusCode != 400 {
		t.Errorf("position 101 status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/nodes/1/position", `not json`, nil); resp.StatusCode != 400 {
		t.Errorf("bad json status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/loxone/node/1/set/abc", "", nil); resp.StatusCode != 400 {
		t.Errorf("set abc status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/loxone/node/x/stop", "", nil); resp.StatusCode != 400 {
		t.Errorf("stop bad id status = %d", resp.StatusCode)
	}

	// Rejected by the gateway -> 502
	srv.SetRejectCommands(true)
	if resp, body := do(t, "GET", ts.URL+"/loxone/node/1/open", "", nil); resp.StatusCode != 502 || body != "ERROR" {
		t.Errorf("rejected: %d %q", resp.StatusCode, body)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/nodes/1/open", "", nil); resp.StatusCode != 502 {
		t.Errorf("rejected api: %d", resp.StatusCode)
	}
	srv.SetRejectCommands(false)

	// Position read reflects the last movement (close -> 100%, stop keeps it)
	waitFor(t, "node 1 at 100%", func() bool { n, _ := gw.GetNode(1); return n.PositionPercent == 100 })
	if resp, body := do(t, "GET", ts.URL+"/loxone/node/1/position", "", nil); resp.StatusCode != 200 || body != "100" {
		t.Errorf("position: %d %q", resp.StatusCode, body)
	}
}

func TestSensorEndpointsConnected(t *testing.T) {
	ts, srv, _, _ := newConnectedServer(t)

	if _, body := do(t, "GET", ts.URL+"/loxone/sensors", "", nil); body != "0;0" {
		t.Errorf("sensors = %q", body)
	}
	srv.SetLimitation(0, klf200.LimitationOriginatorRain)
	srv.SetLimitation(1, klf200.LimitationOriginatorWind)

	resp, body := do(t, "POST", ts.URL+"/api/sensors/refresh", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"rain_detected":true`) || !strings.Contains(body, `"wind_detected":true`) {
		t.Errorf("refresh: %d %s", resp.StatusCode, body)
	}
	if _, body := do(t, "GET", ts.URL+"/api/sensors", "", nil); !strings.Contains(body, `"rain_detected":true`) {
		t.Errorf("sensors json = %s", body)
	}
	if _, body := do(t, "GET", ts.URL+"/loxone/sensors", "", nil); body != "1;1" {
		t.Errorf("sensors = %q", body)
	}
	if _, body := do(t, "GET", ts.URL+"/loxone/sensors/rain", "", nil); body != "1" {
		t.Errorf("rain = %q", body)
	}
	if _, body := do(t, "GET", ts.URL+"/loxone/sensors/wind", "", nil); body != "1" {
		t.Errorf("wind = %q", body)
	}
}

func TestReconnectEndpoint(t *testing.T) {
	ts, srv, _, _ := newConnectedServer(t)
	resp, body := do(t, "POST", ts.URL+"/api/reconnect", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"success":true`) {
		t.Errorf("reconnect: %d %s", resp.StatusCode, body)
	}
	if srv.Connections() != 2 {
		t.Errorf("connections = %d, want 2", srv.Connections())
	}

	srv.Close()
	resp, body = do(t, "POST", ts.URL+"/api/reconnect", "", nil)
	if resp.StatusCode != 502 || !strings.Contains(body, `"success":false`) {
		t.Errorf("reconnect to closed server: %d %s", resp.StatusCode, body)
	}
}

func TestLoxoneUDPConfigAndTest(t *testing.T) {
	ts, _, gw, mgr := newConnectedServer(t)

	// Test without UDP enabled -> 400
	if resp, _ := do(t, "POST", ts.URL+"/api/loxone/config/udp/test", "", nil); resp.StatusCode != 400 {
		t.Errorf("udp test while disabled status = %d", resp.StatusCode)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	port := conn.LocalAddr().(*net.UDPAddr).Port

	body := `{"enabled":true,"ip":"127.0.0.1","port":` + jsonInt(port) + `}`
	resp, out := do(t, "PUT", ts.URL+"/api/loxone/config/udp", body, nil)
	if resp.StatusCode != 200 || !strings.Contains(out, `"success":true`) {
		t.Fatalf("udp config: %d %s", resp.StatusCode, out)
	}
	if !gw.GetUDPSender().IsEnabled() {
		t.Fatal("UDP sender not enabled through API")
	}
	if !mgr.GetConfig().Loxone.UDPFeedback.Enabled {
		t.Error("config not updated")
	}

	_, out = do(t, "GET", ts.URL+"/api/loxone/config", "", nil)
	if !strings.Contains(out, `"enabled":true`) {
		t.Errorf("loxone config = %s", out)
	}

	if resp, _ := do(t, "POST", ts.URL+"/api/loxone/config/udp/test", "", nil); resp.StatusCode != 200 {
		t.Errorf("udp test status = %d", resp.StatusCode)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil || string(buf[:n]) != "test/ping:1" {
		t.Errorf("udp test packet = %q err=%v", buf[:n], err)
	}

	// Validation: enabled without ip -> 400
	if resp, _ := do(t, "PUT", ts.URL+"/api/loxone/config/udp", `{"enabled":true,"ip":"","port":1}`, nil); resp.StatusCode != 400 {
		t.Errorf("invalid udp config status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/loxone/config/udp", `nope`, nil); resp.StatusCode != 400 {
		t.Errorf("bad json status = %d", resp.StatusCode)
	}
}

func jsonInt(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestStaticFilesAndSPAFallback(t *testing.T) {
	// setupRoutes looks for ./web/dist relative to the working directory
	dir := t.TempDir()
	dist := filepath.Join(dir, "web", "dist")
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html><head></head><body>app</body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("console.log(1)"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	ts, _ := newTestServer(t, "")

	resp, body := do(t, "GET", ts.URL+"/", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "app") {
		t.Errorf("index: %d %s", resp.StatusCode, body)
	}
	_, body = do(t, "GET", ts.URL+"/", "", map[string]string{"X-Ingress-Path": "/api/hassio_ingress/abc"})
	if !strings.Contains(body, `<base href="/api/hassio_ingress/abc/" />`) {
		t.Errorf("ingress base tag missing: %s", body)
	}
	resp, body = do(t, "GET", ts.URL+"/assets/app.js", "", nil)
	if resp.StatusCode != 200 || body != "console.log(1)" {
		t.Errorf("asset: %d %q", resp.StatusCode, body)
	}
	resp, body = do(t, "GET", ts.URL+"/some/spa/route", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "app") {
		t.Errorf("spa fallback: %d %s", resp.StatusCode, body)
	}
	// Directory request falls back to index as well
	resp, body = do(t, "GET", ts.URL+"/assets/", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, "app") {
		t.Errorf("dir fallback: %d %s", resp.StatusCode, body)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cfg.Server.WriteTimeout = 0 // exercises the default request timeout

	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop())
	s := NewServer(&cfg.Server, gw, zerolog.Nop(), &memConfigManager{cfg: cfg}, "v")

	// Shutdown before Start is a no-op
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()

	url := "http://127.0.0.1:" + jsonInt(cfg.Server.Port) + "/health"
	waitFor(t, "server up", func() bool {
		resp, err := httpGet(url)
		return err == nil && resp == 200
	})

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Shutdown")
	}
}
