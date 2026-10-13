// Package klf200test provides a simulated KLF-200 gateway for tests. It
// speaks the SLIP-framed KLF-200 protocol over TLS and implements the subset
// of commands used by the client: password authentication, house status
// monitor, get state (keep-alive), node discovery, command send and
// limitation status.
package klf200test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
)

// Node describes a simulated device.
type Node struct {
	ID       uint8
	Name     string
	Type     klf200.NodeType
	Position uint16 // raw 0..0xC800
	// Limitation reported by GW_LIMITATION_STATUS_NTF (0 = none)
	Limitation klf200.LimitationOriginator
}

// Command records a GW_COMMAND_SEND_REQ received by the server.
type Command struct {
	SessionID     uint16
	NodeIDs       []uint8
	MainParameter uint16
}

// Server is a simulated KLF-200.
type Server struct {
	Password string
	// TLSConfig is a client-side TLS configuration that trusts the server.
	TLSConfig *tls.Config

	listener net.Listener
	t        testing.TB

	mu          sync.Mutex
	nodes       []Node
	commands    []Command
	conns       []net.Conn
	rejectCmds  bool
	silent      bool // do not answer GW_GET_STATE_REQ
	noFinished  bool // omit GW_SESSION_FINISHED_NTF
	acceptCount int
	closed      bool
}

// NewServer starts a simulated KLF-200 on 127.0.0.1 with the given password
// and nodes. It is stopped when the test ends.
func NewServer(t testing.TB, password string, nodes ...Node) *Server {
	t.Helper()

	cert, pool := selfSignedCert(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("klf200test: listen: %v", err)
	}

	s := &Server{
		Password: password,
		TLSConfig: &tls.Config{
			RootCAs:    pool,
			ServerName: "klf200test",
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
		},
		listener: ln,
		t:        t,
		nodes:    append([]Node(nil), nodes...),
	}
	go s.acceptLoop()
	t.Cleanup(s.Close)
	return s
}

// Host returns the listen address host.
func (s *Server) Host() string {
	host, _, _ := net.SplitHostPort(s.listener.Addr().String())
	return host
}

// Port returns the listen port.
func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Commands returns a copy of all commands received so far.
func (s *Server) Commands() []Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Command(nil), s.commands...)
}

// Connections returns how many connections were accepted so far.
func (s *Server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acceptCount
}

// SetRejectCommands makes the server answer GW_COMMAND_SEND_REQ with status 0 (rejected).
func (s *Server) SetRejectCommands(reject bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectCmds = reject
}

// SetSilent makes the server ignore GW_GET_STATE_REQ (keep-alive) frames.
func (s *Server) SetSilent(silent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.silent = silent
}

// SetLimitation sets the limitation originator reported for a node.
func (s *Server) SetLimitation(nodeID uint8, origin klf200.LimitationOriginator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].ID == nodeID {
			s.nodes[i].Limitation = origin
		}
	}
}

// DropConnections closes all client connections (simulates a gateway reset).
func (s *Server) DropConnections() {
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

// PushPositionChange sends an unsolicited GW_NODE_STATE_POSITION_CHANGED_NTF
// to all connected clients (house status monitor).
func (s *Server) PushPositionChange(nodeID uint8, position uint16) {
	s.mu.Lock()
	for i := range s.nodes {
		if s.nodes[i].ID == nodeID {
			s.nodes[i].Position = position
		}
	}
	conns := append([]net.Conn(nil), s.conns...)
	s.mu.Unlock()
	frame := klf200.EncodeFrame(klf200.GW_NODE_STATE_POSITION_CHANGED_NTF, positionChangedPayload(nodeID, klf200.NodeStateDone, position, position))
	for _, c := range conns {
		c.Write(frame)
	}
}

// Close stops the server.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	s.listener.Close()
	for _, c := range conns {
		c.Close()
	}
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.acceptCount++
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.serve(conn)
	}
}

func (s *Server) removeConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.conns {
		if c == conn {
			s.conns = append(s.conns[:i], s.conns[i+1:]...)
			return
		}
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	defer s.removeConn(conn)

	authenticated := false
	buf := make([]byte, 1024)
	var frameBuf []byte
	inFrame := false

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				// connection reset etc. - fine for tests
			}
			return
		}
		for i := 0; i < n; i++ {
			b := buf[i]
			if b == klf200.SlipEnd {
				if inFrame && len(frameBuf) > 0 {
					raw := append([]byte{klf200.SlipEnd}, frameBuf...)
					raw = append(raw, klf200.SlipEnd)
					frame, err := klf200.DecodeFrame(raw)
					if err == nil {
						if !s.handle(conn, frame, &authenticated) {
							return
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

func send(conn net.Conn, cmd klf200.CommandID, payload []byte) {
	conn.Write(klf200.EncodeFrame(cmd, payload))
}

// handle processes one request frame. It returns false when the connection
// should be closed.
func (s *Server) handle(conn net.Conn, f *klf200.Frame, authenticated *bool) bool {
	switch f.Command {
	case klf200.GW_PASSWORD_ENTER_REQ:
		pw := string(trimZero(f.Data))
		if pw == s.Password {
			*authenticated = true
			send(conn, klf200.GW_PASSWORD_ENTER_CFM, []byte{0})
		} else {
			send(conn, klf200.GW_PASSWORD_ENTER_CFM, []byte{1})
		}
		return true
	}

	if !*authenticated {
		send(conn, klf200.GW_ERROR_NTF, []byte{3}) // 3 = not authenticated
		return true
	}

	switch f.Command {
	case klf200.GW_HOUSE_STATUS_MONITOR_ENABLE_REQ:
		send(conn, klf200.GW_HOUSE_STATUS_MONITOR_ENABLE_CFM, nil)

	case klf200.GW_GET_STATE_REQ:
		s.mu.Lock()
		silent := s.silent
		s.mu.Unlock()
		if !silent {
			send(conn, klf200.GW_GET_STATE_CFM, []byte{2, 0, 0, 0, 0, 0})
		}

	case klf200.GW_GET_ALL_NODES_INFORMATION_REQ:
		s.mu.Lock()
		nodes := append([]Node(nil), s.nodes...)
		s.mu.Unlock()
		if len(nodes) == 0 {
			send(conn, klf200.GW_GET_ALL_NODES_INFORMATION_CFM, []byte{1, 0})
			return true
		}
		send(conn, klf200.GW_GET_ALL_NODES_INFORMATION_CFM, []byte{0, byte(len(nodes))})
		for _, n := range nodes {
			send(conn, klf200.GW_GET_ALL_NODES_INFORMATION_NTF, nodeInfoPayload(n))
		}
		send(conn, klf200.GW_GET_ALL_NODES_INFORMATION_FINISHED_NTF, nil)

	case klf200.GW_COMMAND_SEND_REQ:
		if len(f.Data) < 66 {
			send(conn, klf200.GW_ERROR_NTF, []byte{1})
			return true
		}
		sessionID := binary.BigEndian.Uint16(f.Data[0:2])
		mainParam := binary.BigEndian.Uint16(f.Data[7:9])
		count := int(f.Data[41])
		nodeIDs := append([]uint8(nil), f.Data[42:42+count]...)

		s.mu.Lock()
		s.commands = append(s.commands, Command{SessionID: sessionID, NodeIDs: nodeIDs, MainParameter: mainParam})
		reject := s.rejectCmds
		s.mu.Unlock()

		if reject {
			send(conn, klf200.GW_COMMAND_SEND_CFM, append(sessionBytes(sessionID), 0))
			return true
		}
		send(conn, klf200.GW_COMMAND_SEND_CFM, append(sessionBytes(sessionID), 1))

		for _, id := range nodeIDs {
			s.mu.Lock()
			var node *Node
			for i := range s.nodes {
				if s.nodes[i].ID == id {
					node = &s.nodes[i]
				}
			}
			if node == nil {
				s.mu.Unlock()
				continue
			}
			if mainParam <= klf200.PositionMax {
				node.Position = mainParam
			}
			pos := node.Position
			limitation := node.Limitation
			s.mu.Unlock()

			statusReply := klf200.StatusReplyCommandCompletedOk
			runStatus := klf200.RunStatusExecutionCompleted
			switch limitation {
			case klf200.LimitationOriginatorRain:
				statusReply, runStatus = klf200.StatusReplyLimitationByRain, klf200.RunStatusExecutionFailed
			case klf200.LimitationOriginatorWind:
				statusReply, runStatus = klf200.StatusReplyLimitationByWind, klf200.RunStatusExecutionFailed
			}
			send(conn, klf200.GW_COMMAND_RUN_STATUS_NTF, runStatusPayload(sessionID, id, pos, runStatus, statusReply))
			send(conn, klf200.GW_NODE_STATE_POSITION_CHANGED_NTF, positionChangedPayload(id, klf200.NodeStateDone, pos, pos))
		}
		send(conn, klf200.GW_SESSION_FINISHED_NTF, sessionBytes(sessionID))

	case klf200.GW_GET_LIMITATION_STATUS_REQ:
		if len(f.Data) < 25 {
			send(conn, klf200.GW_ERROR_NTF, []byte{1})
			return true
		}
		sessionID := binary.BigEndian.Uint16(f.Data[0:2])
		count := int(f.Data[2])
		nodeIDs := f.Data[3 : 3+count]
		send(conn, klf200.GW_GET_LIMITATION_STATUS_CFM, append(sessionBytes(sessionID), 1))
		for _, id := range nodeIDs {
			s.mu.Lock()
			var origin klf200.LimitationOriginator
			for _, n := range s.nodes {
				if n.ID == id {
					origin = n.Limitation
				}
			}
			s.mu.Unlock()
			send(conn, klf200.GW_LIMITATION_STATUS_NTF, limitationPayload(sessionID, id, origin))
		}
		send(conn, klf200.GW_SESSION_FINISHED_NTF, sessionBytes(sessionID))

	default:
		send(conn, klf200.GW_ERROR_NTF, []byte{2}) // 2 = unknown command
	}
	return true
}

func trimZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

func sessionBytes(id uint16) []byte {
	return []byte{byte(id >> 8), byte(id)}
}

func nodeInfoPayload(n Node) []byte {
	p := make([]byte, 124)
	p[0] = n.ID
	copy(p[4:68], n.Name)
	binary.BigEndian.PutUint16(p[69:71], uint16(n.Type))
	p[84] = byte(klf200.NodeStateDone)
	binary.BigEndian.PutUint16(p[85:87], n.Position)
	binary.BigEndian.PutUint16(p[87:89], n.Position)
	return p
}

func positionChangedPayload(id uint8, state klf200.NodeState, pos, target uint16) []byte {
	p := make([]byte, 20)
	p[0] = id
	p[1] = byte(state)
	binary.BigEndian.PutUint16(p[2:4], pos)
	binary.BigEndian.PutUint16(p[4:6], target)
	return p
}

func runStatusPayload(session uint16, id uint8, value uint16, run klf200.RunStatus, reply klf200.StatusReply) []byte {
	p := make([]byte, 13)
	binary.BigEndian.PutUint16(p[0:2], session)
	p[2] = 1
	p[3] = id
	binary.BigEndian.PutUint16(p[5:7], value)
	p[7] = byte(run)
	p[8] = byte(reply)
	return p
}

func limitationPayload(session uint16, id uint8, origin klf200.LimitationOriginator) []byte {
	p := make([]byte, 10)
	binary.BigEndian.PutUint16(p[0:2], session)
	p[2] = id
	if origin == 0 {
		binary.BigEndian.PutUint16(p[4:6], klf200.PositionUnknown)
		binary.BigEndian.PutUint16(p[6:8], klf200.PositionUnknown)
		p[8] = byte(klf200.LimitationOriginatorUser)
		p[9] = 0
		return p
	}
	binary.BigEndian.PutUint16(p[4:6], 0)
	binary.BigEndian.PutUint16(p[6:8], klf200.PositionMax)
	p[8] = byte(origin)
	p[9] = 253 // unlimited
	return p
}

func selfSignedCert(t testing.TB) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "klf200test"},
		DNSNames:              []string{"klf200test"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}, pool
}
