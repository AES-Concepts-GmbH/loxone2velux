package loxone

import (
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
)

// udpListener starts a local UDP listener and returns its port and a channel
// of received messages.
func udpListener(t *testing.T) (int, <-chan string) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	msgs := make(chan string, 16)
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

func recv(t *testing.T, msgs <-chan string) string {
	t.Helper()
	select {
	case m := <-msgs:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no UDP message received")
		return ""
	}
}

func TestUDPSenderSendAndReconfigure(t *testing.T) {
	port, msgs := udpListener(t)
	s := NewUDPSender(zerolog.Nop())

	if s.IsEnabled() {
		t.Fatal("new sender must be disabled")
	}
	s.Send("x", "y", 1) // no-op while disabled
	select {
	case m := <-msgs:
		t.Fatalf("unexpected message while disabled: %s", m)
	case <-time.After(50 * time.Millisecond):
	}

	if err := s.Configure(config.UDPFeedbackConfig{Enabled: true, IP: "127.0.0.1", Port: port}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !s.IsEnabled() {
		t.Fatal("expected enabled")
	}

	s.Send("dachfenster", "position", 42)
	if got := recv(t, msgs); got != "dachfenster/position:42" {
		t.Errorf("got %q", got)
	}
	s.Send("dachfenster", "rain", 1)
	if got := recv(t, msgs); got != "dachfenster/rain:1" {
		t.Errorf("got %q", got)
	}

	// Disabling closes the connection
	if err := s.Configure(config.UDPFeedbackConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if s.IsEnabled() {
		t.Error("expected disabled after reconfigure")
	}

	// Enabled but incomplete config is treated as disabled
	if err := s.Configure(config.UDPFeedbackConfig{Enabled: true, IP: "", Port: port}); err != nil || s.IsEnabled() {
		t.Errorf("incomplete config: err=%v enabled=%v", err, s.IsEnabled())
	}

	// Invalid address is an error
	if err := s.Configure(config.UDPFeedbackConfig{Enabled: true, IP: "not a host name!", Port: port}); err == nil {
		t.Error("expected error for invalid address")
	}

	// Re-enable, then Close
	if err := s.Configure(config.UDPFeedbackConfig{Enabled: true, IP: "127.0.0.1", Port: port}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s.IsEnabled() {
		t.Error("expected disabled after Close")
	}
	s.Close() // idempotent
}
