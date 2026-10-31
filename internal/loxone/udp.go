package loxone

import (
	"fmt"
	"net"
	"sync"

	"github.com/rs/zerolog"
	"github.com/stefanbeyeler/loxone2velux/internal/config"
)

// UDPSender sends status updates to a Loxone Miniserver via UDP
type UDPSender struct {
	conn    *net.UDPConn
	mu      sync.Mutex
	enabled bool
	logger  zerolog.Logger
}

// NewUDPSender creates a new UDP sender (initially disabled)
func NewUDPSender(logger zerolog.Logger) *UDPSender {
	return &UDPSender{
		logger: logger.With().Str("component", "udp-sender").Logger(),
	}
}

// Configure sets up or reconfigures the UDP sender based on current config.
func (s *UDPSender) Configure(cfg config.UDPFeedbackConfig) error {
	commit, abort, err := s.Prepare(cfg)
	if err != nil {
		return err
	}
	defer abort()
	commit()
	return nil
}

// Prepare opens a replacement socket without changing the active sender.
// Commit only after persistence succeeds; abort releases an unused socket.
func (s *UDPSender) Prepare(cfg config.UDPFeedbackConfig) (commit func(), abort func(), err error) {
	var conn *net.UDPConn
	if cfg.Enabled && cfg.IP != "" && cfg.Port != 0 {
		addr, resolveErr := net.ResolveUDPAddr("udp", net.JoinHostPort(cfg.IP, fmt.Sprint(cfg.Port)))
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("invalid UDP address: %w", resolveErr)
		}
		conn, err = net.DialUDP("udp", nil, addr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create UDP connection: %w", err)
		}
	}
	return func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.conn != nil {
				s.conn.Close()
			}
			s.conn = conn
			s.enabled = conn != nil
			conn = nil // Ownership transferred to the sender.
		}, func() {
			if conn != nil {
				conn.Close()
				conn = nil
			}
		}, nil
}

// IsEnabled returns whether UDP feedback is currently enabled
func (s *UDPSender) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled && s.conn != nil
}

// Send sends a single property update to the Loxone Miniserver.
// Format: "<loxone_id>/<property>:<value>"
// Socket errors are logged and returned; UDP delivery remains unacknowledged.
func (s *UDPSender) Send(loxoneID, property string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled || s.conn == nil {
		return fmt.Errorf("UDP feedback disabled")
	}

	msg := fmt.Sprintf("%s/%s:%v", loxoneID, property, value)

	_, err := s.conn.Write([]byte(msg))
	if err != nil {
		s.logger.Warn().Err(err).Str("msg", msg).Msg("Failed to send UDP feedback")
	} else {
		s.logger.Debug().Str("msg", msg).Msg("UDP feedback sent")
	}
	return err
}

// Close closes the UDP connection
func (s *UDPSender) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.enabled = false
}
