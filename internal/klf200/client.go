package klf200

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// Client represents a connection to a KLF-200 gateway
type Client struct {
	host     string
	port     int
	password string
	logger   zerolog.Logger
	tlsCfg   *tls.Config

	// conn is the active connection. It is only ever replaced while connMu
	// is held. The read loop keeps its own reference so it can tell whether
	// the connection it is reading from is still the current one.
	conn          *tls.Conn
	connMu        sync.Mutex
	connected     atomic.Bool
	authenticated atomic.Bool

	sessionID atomic.Uint32

	// Callbacks (set before Connect, read from the read loop)
	onNodeUpdate   func(*Node)
	onSensorUpdate func(SensorStatus)
	onDisconnect   func(error)

	// Response channel for request/response exchanges
	responseChan chan *Frame

	// readDone is closed by the read loop of the current connection when it
	// exits. It is replaced together with conn under connMu, so Disconnect
	// can wait for exactly the loop it is tearing down.
	readDone chan struct{}

	// exchangeMu serializes request/response exchanges that read from
	// responseChan (Authenticate, GetAllNodes, SetPosition, Stop, GetState,
	// GetLimitationStatus) so concurrent callers cannot steal each other's
	// confirmation frames. This is a coarse lock, not per-session demux.
	exchangeMu sync.Mutex

	// Sensor status, derived from the per-node limitation originators.
	sensorStatus    SensorStatus
	nodeLimitations map[uint8]LimitationOriginator
	sensorStatusMu  sync.RWMutex
}

// ClientConfig holds configuration for the KLF-200 client
type ClientConfig struct {
	Host     string
	Port     int
	Password string
	Logger   zerolog.Logger

	// TLSConfig optionally overrides the TLS client configuration. When nil
	// the connection is verified against the embedded Velux CA. Tests use
	// this to talk to a simulated gateway.
	TLSConfig *tls.Config
}

// NewClient creates a new KLF-200 client
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		host:            cfg.Host,
		port:            cfg.Port,
		password:        cfg.Password,
		logger:          cfg.Logger,
		tlsCfg:          cfg.TLSConfig,
		responseChan:    make(chan *Frame, 100),
		nodeLimitations: make(map[uint8]LimitationOriginator),
	}
}

// UpdateConfig updates the client configuration (should be disconnected first)
func (c *Client) UpdateConfig(cfg ClientConfig) {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	c.host = cfg.Host
	c.port = cfg.Port
	c.password = cfg.Password
	if cfg.TLSConfig != nil {
		c.tlsCfg = cfg.TLSConfig
	}
	if cfg.Logger.GetLevel() != zerolog.Disabled {
		c.logger = cfg.Logger
	}
}

// SetNodeUpdateCallback sets the callback for node updates
func (c *Client) SetNodeUpdateCallback(cb func(*Node)) {
	c.onNodeUpdate = cb
}

// SetSensorUpdateCallback sets the callback for sensor status changes
func (c *Client) SetSensorUpdateCallback(cb func(SensorStatus)) {
	c.onSensorUpdate = cb
}

// SetDisconnectCallback sets the callback for disconnection
func (c *Client) SetDisconnectCallback(cb func(error)) {
	c.onDisconnect = cb
}

// veluxCA is the parsed Velux root certificate used to verify the KLF-200.
var veluxCA *x509.Certificate

func init() {
	block, _ := pem.Decode([]byte(VeluxCA))
	if block == nil {
		panic("klf200: embedded Velux CA certificate is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("klf200: embedded Velux CA certificate is invalid: " + err.Error())
	}
	veluxCA = cert
}

// verifyVeluxCertificate verifies the KLF-200 certificate chain against the
// embedded Velux CA.
//
// The KLF-200 certificate has no usable hostname (no CN/SAN matching the IP)
// and the Velux CA expired in 2026, while devices in the field keep using it.
// We therefore verify the signature chain but deliberately ignore hostname
// and validity period: the check is done at a point in time at which the
// presented leaf certificate was valid. A certificate that was not issued by
// the Velux CA is still rejected, which is what protects against a spoofed
// gateway on the LAN.
func verifyVeluxCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errors.New("KLF-200 presented no certificate")
	}

	certs := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return fmt.Errorf("failed to parse KLF-200 certificate: %w", err)
		}
		certs = append(certs, cert)
	}

	roots := x509.NewCertPool()
	roots.AddCert(veluxCA)
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	leaf := certs[0]
	// Candidate verification times: the leaf must have been valid at its own
	// NotBefore; fall back to the CA's NotBefore for oddly dated leaves.
	candidates := []time.Time{
		leaf.NotBefore.Add(time.Second),
		veluxCA.NotBefore.Add(time.Second),
	}

	var lastErr error
	for _, at := range candidates {
		opts := x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   at,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		}
		if _, err := leaf.Verify(opts); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("KLF-200 certificate is not signed by the Velux CA: %w", lastErr)
}

// Connect establishes connection to the KLF-200
func (c *Client) Connect(ctx context.Context) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil && c.connected.Load() {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	c.logger.Info().Str("addr", addr).Msg("Connecting to KLF-200")

	// InsecureSkipVerify disables Go's built-in verification (which would
	// fail on the missing hostname and the expired CA); the chain is instead
	// verified explicitly in verifyVeluxCertificate.
	tlsConfig := c.tlsCfg
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			InsecureSkipVerify:    true, //nolint:gosec // custom chain verification below
			VerifyPeerCertificate: verifyVeluxCertificate,
			MinVersion:            tls.VersionTLS12,
			MaxVersion:            tls.VersionTLS12,
		}
	}

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	// Connect with context
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to KLF-200: %w", err)
	}

	// Wrap with TLS
	conn := tls.Client(netConn, tlsConfig)

	// Perform TLS handshake with timeout
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	if err := conn.HandshakeContext(ctx); err != nil {
		netConn.Close()
		return fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Clear deadline
	conn.SetDeadline(time.Time{})

	// Drop stale frames from a previous connection so they cannot satisfy
	// an exchange on the new one.
	c.drainResponses()

	c.conn = conn
	c.connected.Store(true)

	// Start reader goroutine. Pass the connection explicitly so the loop reads
	// from a local reference instead of racing with Disconnect setting c.conn = nil.
	done := make(chan struct{})
	c.readDone = done
	go c.readLoop(conn, done)

	c.logger.Info().Msg("Connected to KLF-200")

	return nil
}

// drainResponses discards everything currently queued in responseChan.
func (c *Client) drainResponses() {
	for {
		select {
		case <-c.responseChan:
		default:
			return
		}
	}
}

// Authenticate authenticates with the KLF-200
func (c *Client) Authenticate(ctx context.Context) error {
	if !c.connected.Load() {
		return fmt.Errorf("not connected")
	}

	c.exchangeMu.Lock()
	defer c.exchangeMu.Unlock()

	c.logger.Debug().Msg("Authenticating with KLF-200")

	// Send password. Note: the frame embeds the password bytes, so it must
	// never be logged in cleartext.
	frame := BuildPasswordEnterRequest(c.password)
	if err := c.sendRaw(frame); err != nil {
		return fmt.Errorf("failed to send password: %w", err)
	}

	// Wait for response
	resp, err := c.waitForResponse(ctx, GW_PASSWORD_ENTER_CFM, 10*time.Second)
	if err != nil {
		return fmt.Errorf("authentication timeout: %w", err)
	}

	ok, err := ParsePasswordConfirm(resp.Data)
	if err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	if !ok {
		return fmt.Errorf("authentication failed: invalid password")
	}

	c.authenticated.Store(true)
	c.logger.Info().Msg("Authenticated with KLF-200")

	// Enable house status monitor
	if err := c.enableHouseStatusMonitor(ctx); err != nil {
		c.logger.Warn().Err(err).Msg("Failed to enable house status monitor")
	}

	return nil
}

// enableHouseStatusMonitor enables notifications for position changes
func (c *Client) enableHouseStatusMonitor(ctx context.Context) error {
	frame := BuildHouseStatusMonitorEnableRequest()
	if err := c.sendRaw(frame); err != nil {
		return err
	}

	_, err := c.waitForResponse(ctx, GW_HOUSE_STATUS_MONITOR_ENABLE_CFM, 5*time.Second)
	return err
}

// GetState sends GW_GET_STATE_REQ and waits for the confirmation. It is used
// as a keep-alive: the KLF-200 closes connections that stay idle for too
// long, and a missing confirmation reveals a dead connection.
func (c *Client) GetState(ctx context.Context) error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	c.exchangeMu.Lock()
	defer c.exchangeMu.Unlock()

	if err := c.sendRaw(BuildGetStateRequest()); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if _, err := c.waitForResponse(ctx, GW_GET_STATE_CFM, 5*time.Second); err != nil {
		return fmt.Errorf("no response to GW_GET_STATE_REQ: %w", err)
	}
	return nil
}

// GetAllNodes retrieves all nodes from the KLF-200. It returns an error if
// the node list was not received completely, so callers never replace a
// good cache with a truncated list.
func (c *Client) GetAllNodes(ctx context.Context) ([]*Node, error) {
	if !c.authenticated.Load() {
		return nil, fmt.Errorf("not authenticated")
	}

	c.exchangeMu.Lock()
	defer c.exchangeMu.Unlock()

	c.logger.Debug().Msg("Getting all nodes")

	frame := BuildGetAllNodesRequest()
	if err := c.sendRaw(frame); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for confirmation
	cfm, err := c.waitForResponse(ctx, GW_GET_ALL_NODES_INFORMATION_CFM, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to get confirmation: %w", err)
	}
	// Status 0 = OK, 1 = system table empty (no nodes)
	if len(cfm.Data) > 0 && cfm.Data[0] == 1 {
		return []*Node{}, nil
	}

	// Collect node notifications
	var nodes []*Node
	for {
		resp, err := c.waitForResponse(ctx, 0, 5*time.Second) // Accept any response
		if err != nil {
			return nil, fmt.Errorf("node list incomplete (%d nodes received): %w", len(nodes), err)
		}

		switch resp.Command {
		case GW_GET_ALL_NODES_INFORMATION_NTF:
			node, err := ParseNodeInformation(resp.Data)
			if err != nil {
				c.logger.Warn().Err(err).Msg("Failed to parse node info")
				continue
			}
			node.LastUpdate = time.Now()
			nodes = append(nodes, node)
			c.logger.Debug().Uint8("id", node.ID).Str("name", node.Name).Msg("Found node")

		case GW_GET_ALL_NODES_INFORMATION_FINISHED_NTF:
			c.logger.Debug().Int("count", len(nodes)).Msg("Finished getting nodes")
			return nodes, nil
		}
	}
}

// SetPosition sets the position of a node (0-100%)
func (c *Client) SetPosition(ctx context.Context, nodeID uint8, percent float64) error {
	return c.sendMainParameter(ctx, nodeID, PercentToPosition(percent))
}

// sendMainParameter sends a GW_COMMAND_SEND_REQ with the given main parameter
// and waits for the confirmation.
func (c *Client) sendMainParameter(ctx context.Context, nodeID uint8, mainParameter uint16) error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	c.exchangeMu.Lock()
	defer c.exchangeMu.Unlock()

	sessionID := uint16(c.sessionID.Add(1))

	c.logger.Debug().
		Uint8("node", nodeID).
		Uint16("sessionID", sessionID).
		Uint16("mainParameter", mainParameter).
		Msg("Sending command")

	frame := BuildCommandSendRequest(
		sessionID,
		1, // User originated
		PriorityUserLevel2,
		[]uint8{nodeID},
		mainParameter,
		nil,
	)

	if err := c.sendRaw(frame); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Wait for confirmation (GW_COMMAND_SEND_CFM) or error (GW_ERROR_NTF)
	return c.waitForCommandConfirm(ctx, sessionID)
}

// waitForCommandConfirm waits for a GW_COMMAND_SEND_CFM (or GW_ERROR_NTF)
// after a command has been sent. Any other (async) frames that slip into the
// response channel are dispatched via handleAsyncFrame. Callers must hold
// exchangeMu so confirmations are not consumed by a concurrent exchange.
func (c *Client) waitForCommandConfirm(ctx context.Context, sessionID uint16) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("command timeout: %w", ctx.Err())
		case resp := <-c.responseChan:
			switch resp.Command {
			case GW_ERROR_NTF:
				errorCode := uint8(0)
				if len(resp.Data) > 0 {
					errorCode = resp.Data[0]
				}
				c.logger.Error().
					Uint8("errorCode", errorCode).
					Msg("KLF-200 returned error")
				return fmt.Errorf("KLF-200 error: code %d", errorCode)

			case GW_COMMAND_SEND_CFM:
				gotSession, accepted, err := ParseCommandSendConfirm(resp.Data)
				if err != nil {
					return fmt.Errorf("failed to parse response: %w", err)
				}
				if gotSession != sessionID {
					// Stale confirmation from an earlier exchange; keep waiting.
					c.logger.Debug().Uint16("sessionID", gotSession).Msg("Ignoring confirmation for other session")
					continue
				}
				if !accepted {
					return fmt.Errorf("command rejected by KLF-200")
				}
				c.logger.Debug().Uint16("sessionID", sessionID).Msg("Command accepted")
				return nil

			default:
				// Handle async notifications (position changes, etc.)
				c.handleAsyncFrame(resp)
			}
		}
	}
}

// Open fully opens a node (position 0%)
func (c *Client) Open(ctx context.Context, nodeID uint8) error {
	return c.SetPosition(ctx, nodeID, 0)
}

// Close fully closes a node (position 100%)
func (c *Client) Close(ctx context.Context, nodeID uint8) error {
	return c.SetPosition(ctx, nodeID, 100)
}

// Stop stops a node's movement by commanding it to its current position.
func (c *Client) Stop(ctx context.Context, nodeID uint8) error {
	c.logger.Debug().Uint8("node", nodeID).Msg("Stopping node")
	return c.sendMainParameter(ctx, nodeID, PositionCurrent)
}

// GetLimitationStatus requests the limitation status for the given nodes and
// processes the resulting GW_LIMITATION_STATUS_NTF notifications (which
// update the sensor status). Nodes are queried in chunks of 20.
func (c *Client) GetLimitationStatus(ctx context.Context, nodeIDs []uint8) error {
	if !c.authenticated.Load() {
		return fmt.Errorf("not authenticated")
	}

	for start := 0; start < len(nodeIDs); start += MaxNodesPerRequest {
		end := start + MaxNodesPerRequest
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		if err := c.getLimitationStatusChunk(ctx, nodeIDs[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) getLimitationStatusChunk(ctx context.Context, nodeIDs []uint8) error {
	c.exchangeMu.Lock()
	defer c.exchangeMu.Unlock()

	sessionID := uint16(c.sessionID.Add(1))

	c.logger.Debug().Interface("nodes", nodeIDs).Uint16("sessionID", sessionID).Msg("Getting limitation status")

	frame := BuildGetLimitationStatusRequest(sessionID, nodeIDs, 0) // 0 = min limitation
	if err := c.sendRaw(frame); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for confirmation
	cfm, err := c.waitForResponse(ctx, GW_GET_LIMITATION_STATUS_CFM, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get confirmation: %w", err)
	}
	// Status: 0 = rejected, 1 = accepted
	if len(cfm.Data) >= 3 && cfm.Data[2] != 1 {
		return fmt.Errorf("limitation status request rejected by KLF-200")
	}

	// Consume one GW_LIMITATION_STATUS_NTF per requested node. Each one is
	// dispatched through handleAsyncFrame, which updates the sensor status.
	// (GW_SESSION_FINISHED_NTF is handled asynchronously and not relied upon.)
	received := 0
	for {
		resp, err := c.waitForResponse(ctx, 0, 3*time.Second)
		if err != nil {
			if received > 0 {
				// Some gateways do not send GW_SESSION_FINISHED_NTF for this request.
				return nil
			}
			return fmt.Errorf("no limitation status received: %w", err)
		}

		if resp.Command == GW_LIMITATION_STATUS_NTF {
			received++
		}
		c.handleAsyncFrame(resp)
		if received >= len(nodeIDs) {
			return nil
		}
	}
}

// setNodeLimitation records the limitation originator for a node (or clears
// it when origin is 0) and recomputes the aggregated rain/wind status.
func (c *Client) setNodeLimitation(nodeID uint8, origin LimitationOriginator) {
	c.sensorStatusMu.Lock()

	oldRain := c.sensorStatus.RainDetected
	oldWind := c.sensorStatus.WindDetected

	switch origin {
	case LimitationOriginatorRain, LimitationOriginatorWind:
		c.nodeLimitations[nodeID] = origin
	default:
		delete(c.nodeLimitations, nodeID)
	}

	rain, wind := false, false
	for _, o := range c.nodeLimitations {
		switch o {
		case LimitationOriginatorRain:
			rain = true
		case LimitationOriginatorWind:
			wind = true
		}
	}
	c.sensorStatus.RainDetected = rain
	c.sensorStatus.WindDetected = wind
	c.sensorStatus.LastUpdate = time.Now()

	changed := oldRain != rain || oldWind != wind
	statusCopy := c.sensorStatus
	c.sensorStatusMu.Unlock()

	if changed && c.onSensorUpdate != nil {
		c.onSensorUpdate(statusCopy)
	}
}

// updateSensorStatus updates the internal sensor status based on limitation data
func (c *Client) updateSensorStatus(status *LimitationStatus) {
	origin := status.LimitationOrigin
	if !status.Active() {
		origin = 0
	}
	c.setNodeLimitation(status.NodeID, origin)
}

// GetSensorStatus returns the current sensor status
func (c *Client) GetSensorStatus() SensorStatus {
	c.sensorStatusMu.RLock()
	defer c.sensorStatusMu.RUnlock()
	return c.sensorStatus
}

// RefreshSensorStatus queries all nodes for limitation status to update sensor readings.
func (c *Client) RefreshSensorStatus(ctx context.Context, nodeIDs []uint8) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	if err := c.GetLimitationStatus(ctx, nodeIDs); err != nil {
		c.logger.Warn().Err(err).Msg("Sensor status refresh failed, keeping previous values")
		return err
	}
	c.sensorStatusMu.Lock()
	c.sensorStatus.LastUpdate = time.Now()
	c.sensorStatusMu.Unlock()
	return nil
}

// sendRaw sends raw bytes to the KLF-200
func (c *Client) sendRaw(data []byte) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(data)
	return err
}

// waitForResponse waits for a specific response or any response if cmd is 0
func (c *Client) waitForResponse(ctx context.Context, cmd CommandID, timeout time.Duration) (*Frame, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case frame := <-c.responseChan:
			if cmd == 0 || frame.Command == cmd {
				return frame, nil
			}
			// Dispatch other frames (async notifications, stale confirmations)
			c.handleAsyncFrame(frame)
		}
	}
}

// handleAsyncFrame handles frames that were not expected
func (c *Client) handleAsyncFrame(frame *Frame) {
	switch frame.Command {
	case GW_NODE_STATE_POSITION_CHANGED_NTF:
		nodeID, state, position, target, err := ParseNodeStatePositionChangedFull(frame.Data)
		if err != nil {
			c.logger.Warn().Err(err).Msg("Failed to parse position change")
			return
		}
		c.logger.Debug().
			Uint8("nodeID", nodeID).
			Uint8("state", uint8(state)).
			Uint16("position", position).
			Uint16("target", target).
			Float64("percent", PositionToPercent(position)).
			Msg("Node position changed notification")
		if c.onNodeUpdate != nil {
			c.onNodeUpdate(&Node{
				ID:              nodeID,
				State:           state,
				StateStr:        state.String(),
				CurrentPosition: position,
				PositionPercent: PositionToPercent(position),
				TargetPosition:  target,
				TargetPercent:   PositionToPercent(target),
				PositionValid:   true,
				LastUpdate:      time.Now(),
			})
		}

	case GW_COMMAND_RUN_STATUS_NTF:
		sessionID, nodeID, runStatus, statusReply, err := ParseRunStatusNotification(frame.Data)
		if err != nil {
			c.logger.Warn().Err(err).Msg("Failed to parse run status")
			return
		}
		c.logger.Debug().
			Uint16("sessionID", sessionID).
			Uint8("nodeID", nodeID).
			Uint8("runStatus", uint8(runStatus)).
			Uint8("statusReply", uint8(statusReply)).
			Msg("Command run status notification")

		// A command limited by rain/wind tells us the sensor is active for
		// this node; a command that completed normally tells us it is not.
		switch statusReply {
		case StatusReplyLimitationByRain:
			c.logger.Info().Uint8("nodeID", nodeID).Msg("Command limited by rain sensor")
			c.setNodeLimitation(nodeID, LimitationOriginatorRain)
		case StatusReplyLimitationByWind:
			c.logger.Info().Uint8("nodeID", nodeID).Msg("Command limited by wind sensor")
			c.setNodeLimitation(nodeID, LimitationOriginatorWind)
		case StatusReplyCommandCompletedOk:
			c.setNodeLimitation(nodeID, 0)
		}

		// Update state based on run status
		var state NodeState
		switch runStatus {
		case RunStatusExecutionCompleted:
			state = NodeStateDone
		case RunStatusExecutionFailed:
			state = NodeStateErrorWhileExecution
		case RunStatusExecutionActive:
			state = NodeStateExecuting
		default:
			state = NodeStateUnknown
		}
		if c.onNodeUpdate != nil {
			c.onNodeUpdate(&Node{
				ID:         nodeID,
				State:      state,
				StateStr:   state.String(),
				LastUpdate: time.Now(),
			})
		}

	case GW_LIMITATION_STATUS_NTF:
		status, err := ParseLimitationStatusNotification(frame.Data)
		if err != nil {
			c.logger.Warn().Err(err).Msg("Failed to parse limitation status")
			return
		}
		c.logger.Debug().
			Uint8("nodeID", status.NodeID).
			Str("origin", status.LimitationOrigin.String()).
			Bool("active", status.Active()).
			Msg("Limitation status notification")
		c.updateSensorStatus(status)
	}
}

// isAsyncNotification returns true if the frame is an unsolicited
// notification that is handled directly in the read loop instead of being
// queued for a request/response exchange.
func (c *Client) isAsyncNotification(cmd CommandID) bool {
	switch cmd {
	case GW_NODE_STATE_POSITION_CHANGED_NTF, GW_COMMAND_RUN_STATUS_NTF,
		GW_COMMAND_REMAINING_TIME_NTF, GW_SESSION_FINISHED_NTF:
		return true
	default:
		return false
	}
}

// readLoop continuously reads from the TLS connection and extracts SLIP frames.
// It uses the connection passed at startup rather than c.conn so it does not
// race with Disconnect, which sets c.conn = nil. The loop ends when the
// connection is closed (by the peer or by Disconnect).
func (c *Client) readLoop(conn *tls.Conn, done chan struct{}) {
	defer close(done)

	buf := make([]byte, 1024)
	frameBuf := make([]byte, 0, 256)
	inFrame := false

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := conn.Read(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Read timeout is normal, continue
				continue
			}
			if errors.Is(err, io.EOF) {
				c.logger.Info().Msg("Connection closed by KLF-200")
			} else if !errors.Is(err, net.ErrClosed) {
				c.logger.Error().Err(err).Msg("Read error")
			}
			c.handleReadError(conn, err)
			return
		}

		// Process received bytes and extract SLIP frames
		for i := 0; i < n; i++ {
			b := buf[i]

			if b == SlipEnd {
				if inFrame && len(frameBuf) > 0 {
					// End of frame - process it
					frameData := make([]byte, 0, len(frameBuf)+2)
					frameData = append(frameData, SlipEnd)
					frameData = append(frameData, frameBuf...)
					frameData = append(frameData, SlipEnd)

					frame, err := DecodeFrame(frameData)
					if err != nil {
						c.logger.Warn().Err(err).Msg("Failed to decode frame")
					} else {
						c.logger.Debug().
							Uint16("cmd", uint16(frame.Command)).
							Int("dataLen", len(frame.Data)).
							Msg("Received frame")

						// Handle async notifications directly, don't queue them
						if c.isAsyncNotification(frame.Command) {
							c.handleAsyncFrame(frame)
						} else {
							select {
							case c.responseChan <- frame:
							default:
								c.logger.Warn().Msg("Response channel full, dropping frame")
							}
						}
					}
					frameBuf = frameBuf[:0]
				}
				inFrame = true
			} else if inFrame {
				frameBuf = append(frameBuf, b)
			}
		}
	}
}

// handleReadError is called by the read loop when its connection died. It
// only tears down the client state if that connection is still the current
// one; a loop that was superseded by a reconnect must not clear the flags of
// the new connection.
func (c *Client) handleReadError(conn *tls.Conn, err error) {
	c.connMu.Lock()
	if c.conn != conn {
		c.connMu.Unlock()
		return
	}
	c.conn = nil
	c.connected.Store(false)
	c.authenticated.Store(false)
	c.connMu.Unlock()

	conn.Close()

	if c.onDisconnect != nil {
		c.onDisconnect(err)
	}
}

// Disconnect closes the connection and waits for the read loop to finish.
func (c *Client) Disconnect() error {
	c.connMu.Lock()
	conn := c.conn
	done := c.readDone
	c.conn = nil
	c.readDone = nil
	c.connected.Store(false)
	c.authenticated.Store(false)
	c.connMu.Unlock()

	if conn == nil {
		return nil
	}
	err := conn.Close()

	// Wait for this connection's read loop outside the lock (it needs connMu
	// to notice that it has been superseded).
	if done != nil {
		<-done
	}
	return err
}

// IsConnected returns true if connected
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// IsAuthenticated returns true if authenticated
func (c *Client) IsAuthenticated() bool {
	return c.authenticated.Load()
}
