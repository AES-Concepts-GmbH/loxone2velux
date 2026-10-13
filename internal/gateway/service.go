package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
	"github.com/stefanbeyeler/loxone2velux/internal/loxone"
)

// ErrNotConfigured is returned when no KLF-200 host/password is configured.
var ErrNotConfigured = errors.New("KLF-200 not configured (host or password missing)")

// ErrNotConnected is returned by commands while there is no authenticated
// KLF-200 session.
var ErrNotConnected = errors.New("not connected to KLF-200")

// keepAliveInterval is how often GW_GET_STATE_REQ is sent on an idle
// connection. The KLF-200 drops connections without traffic; pyvlx uses
// 30s, we stay below the gateway's limit with a little more headroom.
// (Variable so tests can shorten it.)
var keepAliveInterval = 60 * time.Second

// keepAliveTimeout bounds a single keep-alive exchange.
var keepAliveTimeout = 10 * time.Second

const (
	// keepAliveFailuresBeforeDrop is the number of consecutive keep-alive
	// failures after which the connection is considered dead.
	keepAliveFailuresBeforeDrop = 2

	operationTimeout = 30 * time.Second
)

// Service is the main gateway service
type Service struct {
	cfg            *config.KLF200Config
	client         *klf200.Client
	nodes          *klf200.NodeManager
	udpSender      *loxone.UDPSender
	mappingManager *loxone.MappingManager
	logger         zerolog.Logger

	mu       sync.RWMutex
	started  bool
	stopChan chan struct{}
	// per-loop reload signals (buffered, one per loop that has a ticker)
	reloadRefresh   chan struct{}
	reloadReconnect chan struct{}
	wg              sync.WaitGroup
}

// Option customizes a Service at construction time.
type Option func(*klf200.ClientConfig)

// WithTLSConfig overrides the TLS configuration used towards the KLF-200
// (used by tests with a simulated gateway).
func WithTLSConfig(tlsCfg *tls.Config) Option {
	return func(c *klf200.ClientConfig) { c.TLSConfig = tlsCfg }
}

// NewService creates a new gateway service
func NewService(cfg *config.KLF200Config, loxoneCfg *config.LoxoneConfig, logger zerolog.Logger, opts ...Option) *Service {
	cfgCopy := *cfg
	clientCfg := klf200.ClientConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Password: cfg.Password,
		Logger:   logger.With().Str("component", "klf200-client").Logger(),
	}
	for _, opt := range opts {
		opt(&clientCfg)
	}

	udpSender := loxone.NewUDPSender(logger)
	mappingMgr := loxone.NewMappingManager()

	if loxoneCfg != nil {
		if err := udpSender.Configure(loxoneCfg.UDPFeedback); err != nil {
			logger.Error().Err(err).Msg("Failed to configure UDP sender")
		}
		mappingMgr.Load(loxoneCfg.Mappings)
	}

	return &Service{
		cfg:             &cfgCopy,
		client:          klf200.NewClient(clientCfg),
		nodes:           klf200.NewNodeManager(),
		udpSender:       udpSender,
		mappingManager:  mappingMgr,
		logger:          logger.With().Str("component", "gateway").Logger(),
		stopChan:        make(chan struct{}),
		reloadRefresh:   make(chan struct{}, 1),
		reloadReconnect: make(chan struct{}, 1),
	}
}

// config returns a snapshot of the current KLF-200 configuration.
func (s *Service) config() config.KLF200Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.cfg
}

// IsConfigured reports whether host and password are set.
func (s *Service) IsConfigured() bool {
	cfg := s.config()
	return cfg.Host != "" && cfg.Password != ""
}

// Start starts the gateway service. The background loops (refresh,
// reconnect, keep-alive) are always started so that a KLF-200 configured
// later at runtime is picked up; the initial connection is only attempted
// when host and password are set.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	cfg := s.config()
	s.logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Msg("Starting gateway service")

	// Set callbacks
	s.client.SetNodeUpdateCallback(s.handleNodeUpdate)
	s.client.SetSensorUpdateCallback(s.handleSensorUpdate)
	s.client.SetDisconnectCallback(s.handleDisconnect)

	// Try initial connection (non-blocking on failure)
	var connectErr error
	if s.IsConfigured() {
		if err := s.connect(ctx); err != nil {
			connectErr = err
			s.logger.Warn().Err(err).Msg("Initial connection failed, will retry in background")
		}
	} else {
		s.logger.Warn().Msg("KLF-200 not configured (host or password missing) - waiting for configuration")
	}

	s.wg.Add(3)
	go s.refreshLoop()
	go s.reconnectLoop()
	go s.keepAliveLoop()

	return connectErr
}

// connect establishes connection to KLF-200
func (s *Service) connect(ctx context.Context) error {
	if !s.IsConfigured() {
		return ErrNotConfigured
	}

	if err := s.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if err := s.client.Authenticate(ctx); err != nil {
		s.client.Disconnect()
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Get initial nodes
	if err := s.refreshNodes(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to get initial nodes")
	} else if err := s.RefreshSensorStatus(ctx); err != nil {
		s.logger.Debug().Err(err).Msg("Initial sensor status refresh failed")
	}

	return nil
}

// refreshNodes retrieves all nodes from KLF-200
func (s *Service) refreshNodes(ctx context.Context) error {
	nodes, err := s.client.GetAllNodes(ctx)
	if err != nil {
		return err
	}

	s.nodes.SetNodes(nodes)
	s.logger.Info().Int("count", len(nodes)).Msg("Refreshed nodes")

	return nil
}

// refreshLoop periodically refreshes node information and sensor status.
func (s *Service) refreshLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config().RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.reloadRefresh:
			ticker.Reset(s.config().RefreshInterval)
		case <-ticker.C:
			if !s.client.IsAuthenticated() {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
			if err := s.refreshNodes(ctx); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to refresh nodes")
			} else if err := s.RefreshSensorStatus(ctx); err != nil {
				s.logger.Debug().Err(err).Msg("Failed to refresh sensor status")
			}
			cancel()
		}
	}
}

// reconnectLoop handles reconnection
func (s *Service) reconnectLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config().ReconnectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.reloadReconnect:
			ticker.Reset(s.config().ReconnectInterval)
		case <-ticker.C:
			if s.client.IsConnected() || !s.IsConfigured() {
				continue
			}
			s.logger.Info().Msg("Attempting to reconnect")
			ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
			if err := s.connect(ctx); err != nil {
				s.logger.Warn().Err(err).Msg("Reconnect failed")
			} else {
				s.logger.Info().Msg("Reconnected successfully")
			}
			cancel()
		}
	}
}

// keepAliveLoop sends periodic GW_GET_STATE_REQ frames so the KLF-200 does
// not drop the idle connection, and tears the connection down when the
// gateway stops answering (the reconnect loop then re-establishes it).
func (s *Service) keepAliveLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	failures := 0
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if !s.client.IsAuthenticated() {
				failures = 0
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), keepAliveTimeout)
			err := s.client.GetState(ctx)
			cancel()
			if err == nil {
				failures = 0
				continue
			}
			failures++
			s.logger.Warn().Err(err).Int("failures", failures).Msg("Keep-alive failed")
			if failures >= keepAliveFailuresBeforeDrop {
				s.logger.Warn().Msg("KLF-200 not responding, dropping connection")
				s.client.Disconnect()
				failures = 0
			}
		}
	}
}

// handleNodeUpdate handles node position updates
func (s *Service) handleNodeUpdate(node *klf200.Node) {
	s.nodes.UpdateNode(node)
	s.logger.Debug().
		Uint8("id", node.ID).
		Float64("position", node.PositionPercent).
		Msg("Node position updated")

	s.sendNodeUDPFeedback(node)
}

// sendNodeUDPFeedback sends position/state updates for a node via UDP
func (s *Service) sendNodeUDPFeedback(node *klf200.Node) {
	if !s.udpSender.IsEnabled() {
		return
	}

	mapping := s.mappingManager.GetByNodeID(node.ID)
	if mapping == nil {
		return
	}

	id := mapping.LoxoneID
	// Only forward position/target when the update carries real position data.
	// State-only updates (run status) would otherwise report a bogus 0%.
	if node.PositionValid {
		s.udpSender.Send(id, "position", int(math.Round(node.PositionPercent)))
		s.udpSender.Send(id, "target", int(math.Round(node.TargetPercent)))
	}
	s.udpSender.Send(id, "state", int(node.State))
}

// handleSensorUpdate handles sensor status changes and sends UDP feedback
func (s *Service) handleSensorUpdate(status klf200.SensorStatus) {
	s.logger.Info().
		Bool("rain", status.RainDetected).
		Bool("wind", status.WindDetected).
		Msg("Sensor status changed")

	if !s.udpSender.IsEnabled() {
		return
	}

	rain := 0
	if status.RainDetected {
		rain = 1
	}
	wind := 0
	if status.WindDetected {
		wind = 1
	}

	for _, mapping := range s.mappingManager.GetAll() {
		if !mapping.Enabled {
			continue
		}
		s.udpSender.Send(mapping.LoxoneID, "rain", rain)
		s.udpSender.Send(mapping.LoxoneID, "wind", wind)
	}
}

// handleDisconnect handles disconnection
func (s *Service) handleDisconnect(err error) {
	if err != nil {
		s.logger.Warn().Err(err).Msg("Disconnected from KLF-200")
	} else {
		s.logger.Info().Msg("Disconnected from KLF-200")
	}
}

// Stop stops the gateway service
func (s *Service) Stop() error {
	s.logger.Info().Msg("Stopping gateway service")

	s.mu.Lock()
	select {
	case <-s.stopChan:
		// already stopped
	default:
		close(s.stopChan)
	}
	s.mu.Unlock()

	s.wg.Wait()

	s.udpSender.Close()

	return s.client.Disconnect()
}

// GetUDPSender returns the UDP sender
func (s *Service) GetUDPSender() *loxone.UDPSender {
	return s.udpSender
}

// GetMappingManager returns the mapping manager
func (s *Service) GetMappingManager() *loxone.MappingManager {
	return s.mappingManager
}

// IsConnected returns true if connected to KLF-200
func (s *Service) IsConnected() bool {
	return s.client.IsAuthenticated()
}

// GetNodes returns all nodes
func (s *Service) GetNodes() []*klf200.Node {
	return s.nodes.GetAllNodes()
}

// GetNode returns a node by ID
func (s *Service) GetNode(id uint8) (*klf200.Node, bool) {
	return s.nodes.GetNode(id)
}

// GetNodeCount returns the number of nodes
func (s *Service) GetNodeCount() int {
	return s.nodes.NodeCount()
}

// SetPosition sets the position of a node
func (s *Service) SetPosition(ctx context.Context, nodeID uint8, percent float64) error {
	if !s.client.IsAuthenticated() {
		return ErrNotConnected
	}

	return s.client.SetPosition(ctx, nodeID, percent)
}

// Open fully opens a node
func (s *Service) Open(ctx context.Context, nodeID uint8) error {
	if !s.client.IsAuthenticated() {
		return ErrNotConnected
	}

	return s.client.Open(ctx, nodeID)
}

// Close fully closes a node
func (s *Service) Close(ctx context.Context, nodeID uint8) error {
	if !s.client.IsAuthenticated() {
		return ErrNotConnected
	}

	return s.client.Close(ctx, nodeID)
}

// StopNode stops a node's movement
func (s *Service) StopNode(ctx context.Context, nodeID uint8) error {
	if !s.client.IsAuthenticated() {
		return ErrNotConnected
	}

	return s.client.Stop(ctx, nodeID)
}

// GetSensorStatus returns the current sensor status
func (s *Service) GetSensorStatus() klf200.SensorStatus {
	return s.client.GetSensorStatus()
}

// RefreshSensorStatus queries the KLF-200 for current sensor/limitation status
func (s *Service) RefreshSensorStatus(ctx context.Context) error {
	if !s.client.IsAuthenticated() {
		return ErrNotConnected
	}

	// Get all node IDs
	nodes := s.nodes.GetAllNodes()
	nodeIDs := make([]uint8, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}

	return s.client.RefreshSensorStatus(ctx, nodeIDs)
}

// Reconnect disconnects and reconnects to the KLF-200
func (s *Service) Reconnect(ctx context.Context) error {
	s.logger.Info().Msg("Manual reconnect requested")

	// Disconnect if connected
	if s.client.IsConnected() {
		s.client.Disconnect()
	}

	// Reconnect
	if err := s.connect(ctx); err != nil {
		return fmt.Errorf("reconnect failed: %w", err)
	}

	s.logger.Info().Msg("Reconnected successfully")
	return nil
}

// UpdateConfig updates the KLF-200 configuration. Connection settings take
// effect on the next (re)connect, interval changes are applied immediately.
func (s *Service) UpdateConfig(cfg *config.KLF200Config) {
	cfgCopy := *cfg

	s.mu.Lock()
	s.cfg = &cfgCopy
	s.mu.Unlock()

	// Update client config
	clientCfg := klf200.ClientConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Password: cfg.Password,
		Logger:   s.logger.With().Str("component", "klf200-client").Logger(),
	}
	s.client.UpdateConfig(clientCfg)

	// Wake the loops so they pick up new intervals
	for _, ch := range []chan struct{}{s.reloadRefresh, s.reloadReconnect} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
