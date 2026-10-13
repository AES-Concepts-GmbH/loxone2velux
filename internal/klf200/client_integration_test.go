package klf200_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

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

func newClient(t *testing.T, srv *klf200test.Server, password string) *klf200.Client {
	t.Helper()
	c := klf200.NewClient(klf200.ClientConfig{
		Host:      srv.Host(),
		Port:      srv.Port(),
		Password:  password,
		Logger:    zerolog.Nop(),
		TLSConfig: srv.TLSConfig,
	})
	t.Cleanup(func() { c.Disconnect() })
	return c
}

func connect(t *testing.T, srv *klf200test.Server) *klf200.Client {
	t.Helper()
	c := newClient(t, srv, testPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	return c
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

func TestConnectAuthenticateAndState(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)

	if !c.IsConnected() || !c.IsAuthenticated() {
		t.Fatal("expected connected and authenticated")
	}
	// Connect is idempotent while connected
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	if srv.Connections() != 1 {
		t.Errorf("connections = %d, want 1", srv.Connections())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.GetState(ctx); err != nil {
		t.Errorf("GetState: %v", err)
	}

	if err := c.Disconnect(); err != nil {
		t.Errorf("Disconnect: %v", err)
	}
	if c.IsConnected() || c.IsAuthenticated() {
		t.Error("flags not cleared after Disconnect")
	}
	if err := c.GetState(ctx); err == nil {
		t.Error("GetState after Disconnect should fail")
	}
}

func TestAuthenticateWrongPassword(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := newClient(t, srv, "wrong")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Authenticate(ctx); err == nil {
		t.Fatal("expected authentication failure")
	}
	if c.IsAuthenticated() {
		t.Error("authenticated flag set after failure")
	}
	if _, err := c.GetAllNodes(ctx); err == nil {
		t.Error("GetAllNodes without authentication should fail")
	}
}

func TestConnectRefused(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword)
	srv.Close()
	c := newClient(t, srv, testPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err == nil {
		t.Fatal("expected connection error")
	}
	if err := c.Authenticate(ctx); err == nil {
		t.Error("Authenticate without connection should fail")
	}
}

func TestConnectRejectsUntrustedCertificate(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword)
	// Default client config verifies against the Velux CA, which the fake
	// server cannot present.
	c := klf200.NewClient(klf200.ClientConfig{Host: srv.Host(), Port: srv.Port(), Password: testPassword, Logger: zerolog.Nop()})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err == nil {
		c.Disconnect()
		t.Fatal("connection to a gateway without Velux certificate was accepted")
	}
}

func TestGetAllNodes(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes, err := c.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	if nodes[0].Name != "Dachfenster" || nodes[0].NodeTypeStr != "Window Opener (rain sensor)" || nodes[0].PositionPercent != 0 {
		t.Errorf("node 0 = %+v", nodes[0])
	}
	if nodes[1].Name != "Rollladen" || nodes[1].PositionPercent != 100 {
		t.Errorf("node 1 = %+v", nodes[1])
	}
}

func TestGetAllNodesEmpty(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword)
	c := connect(t, srv)
	nodes, err := c.GetAllNodes(context.Background())
	if err != nil || len(nodes) != 0 {
		t.Fatalf("nodes=%v err=%v", nodes, err)
	}
}

func TestCommands(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)

	var updates atomic.Int32
	var last atomic.Value
	c.SetNodeUpdateCallback(func(n *klf200.Node) {
		updates.Add(1)
		if n.PositionValid {
			last.Store(n)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.SetPosition(ctx, 0, 50); err != nil {
		t.Fatalf("SetPosition: %v", err)
	}
	if err := c.Open(ctx, 1); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := c.Close(ctx, 1); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Stop(ctx, 1); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	cmds := srv.Commands()
	if len(cmds) != 4 {
		t.Fatalf("server received %d commands, want 4", len(cmds))
	}
	want := []uint16{klf200.PercentToPosition(50), klf200.PositionMin, klf200.PositionMax, klf200.PositionCurrent}
	for i, w := range want {
		if cmds[i].MainParameter != w {
			t.Errorf("command %d main parameter = 0x%04x, want 0x%04x", i, cmds[i].MainParameter, w)
		}
	}
	if cmds[0].NodeIDs[0] != 0 || cmds[1].NodeIDs[0] != 1 {
		t.Errorf("node ids: %+v", cmds)
	}
	// Session IDs must be distinct
	if cmds[0].SessionID == cmds[1].SessionID {
		t.Error("session ids not incremented")
	}

	// Position change notifications arrive asynchronously
	waitFor(t, "position notification", func() bool {
		n, _ := last.Load().(*klf200.Node)
		return n != nil && n.ID == 1 && n.PositionPercent == 100
	})
	if updates.Load() < 4 {
		t.Errorf("only %d node updates", updates.Load())
	}
}

func TestCommandRejected(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	srv.SetRejectCommands(true)
	c := connect(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.SetPosition(ctx, 0, 10); err == nil {
		t.Fatal("rejected command reported as success")
	}
}

func TestCommandTimeout(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	srv.SetSilent(true)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := c.GetState(ctx); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestLimitationStatusUpdatesSensors(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	srv.SetLimitation(0, klf200.LimitationOriginatorRain)
	c := connect(t, srv)

	var changes atomic.Int32
	c.SetSensorUpdateCallback(func(klf200.SensorStatus) { changes.Add(1) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.RefreshSensorStatus(ctx, []uint8{0, 1}); err != nil {
		t.Fatalf("RefreshSensorStatus: %v", err)
	}
	st := c.GetSensorStatus()
	if !st.RainDetected || st.WindDetected {
		t.Fatalf("sensor status = %+v", st)
	}
	if changes.Load() != 1 {
		t.Errorf("sensor callback fired %d times, want 1", changes.Load())
	}

	// Rain clears, wind on the other node
	srv.SetLimitation(0, 0)
	srv.SetLimitation(1, klf200.LimitationOriginatorWind)
	if err := c.RefreshSensorStatus(ctx, []uint8{0, 1}); err != nil {
		t.Fatal(err)
	}
	st = c.GetSensorStatus()
	if st.RainDetected || !st.WindDetected {
		t.Fatalf("sensor status after change = %+v", st)
	}

	// Empty node list is a no-op
	if err := c.RefreshSensorStatus(ctx, nil); err != nil {
		t.Error(err)
	}

	// More than 20 nodes are queried in chunks
	many := make([]uint8, 25)
	for i := range many {
		many[i] = uint8(i)
	}
	if err := c.RefreshSensorStatus(ctx, many); err != nil {
		t.Errorf("chunked refresh: %v", err)
	}
}

func TestRunStatusLimitationFromCommand(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	srv.SetLimitation(0, klf200.LimitationOriginatorWind)
	c := connect(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Open(ctx, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "wind from run status", func() bool { return c.GetSensorStatus().WindDetected })
}

func TestServerDropsConnection(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)

	var disconnected atomic.Bool
	c.SetDisconnectCallback(func(error) { disconnected.Store(true) })

	srv.DropConnections()
	waitFor(t, "disconnect callback", disconnected.Load)
	if c.IsConnected() || c.IsAuthenticated() {
		t.Error("flags not cleared after server dropped the connection")
	}

	// Reconnect works on the same client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("re-authenticate: %v", err)
	}
	if _, err := c.GetAllNodes(ctx); err != nil {
		t.Errorf("GetAllNodes after reconnect: %v", err)
	}
}

func TestReconnectDoesNotClearNewConnection(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		if err := c.Disconnect(); err != nil {
			t.Fatal(err)
		}
		if err := c.Connect(ctx); err != nil {
			t.Fatal(err)
		}
		if err := c.Authenticate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Give any superseded read loop time to misbehave
	time.Sleep(100 * time.Millisecond)
	if !c.IsAuthenticated() {
		t.Fatal("a superseded read loop cleared the new connection's state")
	}
	if err := c.GetState(ctx); err != nil {
		t.Errorf("GetState after reconnects: %v", err)
	}
}

func TestUnsolicitedPositionChange(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	var got atomic.Value
	c.SetNodeUpdateCallback(func(n *klf200.Node) { got.Store(n) })

	srv.PushPositionChange(1, klf200.PercentToPosition(25))
	waitFor(t, "pushed position", func() bool {
		n, _ := got.Load().(*klf200.Node)
		return n != nil && n.ID == 1 && n.PositionPercent == 25 && n.PositionValid
	})
}

func TestUpdateConfig(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := newClient(t, srv, "initially-wrong")
	c.UpdateConfig(klf200.ClientConfig{Host: srv.Host(), Port: srv.Port(), Password: testPassword, Logger: zerolog.Nop()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Authenticate(ctx); err != nil {
		t.Fatalf("Authenticate with updated password: %v", err)
	}
}
