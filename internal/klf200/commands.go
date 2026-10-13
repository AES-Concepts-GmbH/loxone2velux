package klf200

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// SLIP protocol constants
const (
	SlipEnd    byte = 0xC0
	SlipEsc    byte = 0xDB
	SlipEscEnd byte = 0xDC
	SlipEscEsc byte = 0xDD
)

// KLF200 protocol constants
const (
	ProtocolID byte = 0x00 // Protocol identifier for KLF-200

	// MaxNodesPerRequest is the size of the index array in command and
	// limitation requests.
	MaxNodesPerRequest = 20
)

var (
	ErrInvalidFrame     = errors.New("invalid frame")
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrFrameTooShort    = errors.New("frame too short")
)

// EncodeFrame creates a SLIP-encoded frame from command and data
// Frame structure: ProtocolID | Length | Command(2) | Data | CRC
func EncodeFrame(cmd CommandID, data []byte) []byte {
	buf := new(bytes.Buffer)

	// Protocol ID (always 0x00 for KLF-200)
	buf.WriteByte(ProtocolID)

	// Protocol length (command + data, excluding protocol ID, length byte, and checksum)
	buf.WriteByte(byte(len(data) + 3))

	// Command ID (big endian)
	binary.Write(buf, binary.BigEndian, uint16(cmd))

	// Data
	buf.Write(data)

	// Calculate checksum (XOR of all bytes including protocol ID)
	frame := buf.Bytes()
	checksum := byte(0)
	for _, b := range frame {
		checksum ^= b
	}
	buf.WriteByte(checksum)

	// SLIP encode
	return slipEncode(buf.Bytes())
}

// slipEncode applies SLIP encoding to data
func slipEncode(data []byte) []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(SlipEnd)

	for _, b := range data {
		switch b {
		case SlipEnd:
			buf.WriteByte(SlipEsc)
			buf.WriteByte(SlipEscEnd)
		case SlipEsc:
			buf.WriteByte(SlipEsc)
			buf.WriteByte(SlipEscEsc)
		default:
			buf.WriteByte(b)
		}
	}

	buf.WriteByte(SlipEnd)
	return buf.Bytes()
}

// slipDecode removes SLIP encoding from data
func slipDecode(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, ErrFrameTooShort
	}

	// Remove leading and trailing END markers
	start := 0
	end := len(data)

	for start < end && data[start] == SlipEnd {
		start++
	}
	for end > start && data[end-1] == SlipEnd {
		end--
	}

	if start >= end {
		return nil, ErrInvalidFrame
	}

	data = data[start:end]
	buf := new(bytes.Buffer)

	for i := 0; i < len(data); i++ {
		if data[i] == SlipEsc {
			if i+1 >= len(data) {
				return nil, ErrInvalidFrame
			}
			switch data[i+1] {
			case SlipEscEnd:
				buf.WriteByte(SlipEnd)
			case SlipEscEsc:
				buf.WriteByte(SlipEsc)
			default:
				return nil, ErrInvalidFrame
			}
			i++
		} else {
			buf.WriteByte(data[i])
		}
	}

	return buf.Bytes(), nil
}

// DecodeFrame parses a SLIP-encoded frame
// Frame structure: ProtocolID | Length | Command(2) | Data | CRC
func DecodeFrame(raw []byte) (*Frame, error) {
	data, err := slipDecode(raw)
	if err != nil {
		return nil, err
	}

	if len(data) < 5 { // ProtocolID + Length + Command(2) + CRC
		return nil, ErrFrameTooShort
	}

	// Verify checksum (XOR of all bytes except last)
	checksum := byte(0)
	for _, b := range data[:len(data)-1] {
		checksum ^= b
	}
	if checksum != data[len(data)-1] {
		return nil, ErrChecksumMismatch
	}

	// Verify protocol ID
	if data[0] != ProtocolID {
		return nil, fmt.Errorf("invalid protocol ID: %02x", data[0])
	}

	// Verify declared length (command + data)
	if int(data[1]) != len(data)-2 {
		return nil, fmt.Errorf("%w: declared length %d, actual %d", ErrInvalidFrame, data[1], len(data)-2)
	}

	// Parse frame
	// data[0] = Protocol ID
	// data[1] = Length (command + data length)
	// data[2:4] = Command ID
	// data[4:len-1] = Data
	// data[len-1] = CRC
	cmd := CommandID(binary.BigEndian.Uint16(data[2:4]))

	frame := &Frame{
		Command: cmd,
		Data:    data[4 : len(data)-1],
	}

	return frame, nil
}

// BuildPasswordEnterRequest creates a password authentication request
// If the password starts with "base64:", the remainder will be decoded
func BuildPasswordEnterRequest(password string) []byte {
	// Password is max 32 bytes, padded with zeros
	data := make([]byte, 32)

	// Only decode as Base64 if explicitly prefixed with "base64:"
	if len(password) > 7 && password[:7] == "base64:" {
		if decoded, err := base64.StdEncoding.DecodeString(password[7:]); err == nil && len(decoded) <= 32 {
			copy(data, decoded)
		} else {
			copy(data, []byte(password))
		}
	} else {
		copy(data, []byte(password))
	}

	return EncodeFrame(GW_PASSWORD_ENTER_REQ, data)
}

// BuildGetAllNodesRequest creates a request to get all node information
func BuildGetAllNodesRequest() []byte {
	return EncodeFrame(GW_GET_ALL_NODES_INFORMATION_REQ, nil)
}

// BuildGetStateRequest creates a GW_GET_STATE_REQ frame. It carries no payload
// and is used as a keep-alive, since the KLF-200 drops idle connections.
func BuildGetStateRequest() []byte {
	return EncodeFrame(GW_GET_STATE_REQ, nil)
}

// BuildCommandSendRequest creates a command to control a node
// Frame structure (66 bytes payload per pyvlx):
// - SessionID: 2 bytes (index 0-1)
// - CommandOriginator: 1 byte (index 2)
// - PriorityLevel: 1 byte (index 3)
// - ParameterActive: 1 byte (index 4) - bit 0 = main parameter active
// - FPI1: 1 byte (index 5) - functional parameter indicator 1
// - FPI2: 1 byte (index 6) - functional parameter indicator 2
// - MainParameter: 2 bytes (index 7-8)
// - FP1-FP16: 32 bytes (index 9-40, 16 x 2 bytes)
// - IndexArrayCount: 1 byte (index 41)
// - IndexArray: 20 bytes (index 42-61)
// - PriorityLevelLock: 1 byte (index 62)
// - PriorityLevelInfo: 2 bytes (index 63-64)
// - LockTime: 1 byte (index 65)
// Total: 66 bytes
func BuildCommandSendRequest(sessionID uint16, commandOriginator uint8, priorityLevel Priority,
	nodeIDs []uint8, mainParameter uint16, functionalParameters []uint16) []byte {

	if len(nodeIDs) > MaxNodesPerRequest {
		nodeIDs = nodeIDs[:MaxNodesPerRequest]
	}

	buf := new(bytes.Buffer)

	// Session ID (2 bytes)
	binary.Write(buf, binary.BigEndian, sessionID)

	// Command originator (1 byte) - typically 1 for user
	buf.WriteByte(commandOriginator)

	// Priority level (1 byte)
	buf.WriteByte(byte(priorityLevel))

	// Parameter active flags (1 byte) - bit 0 = main parameter
	buf.WriteByte(0x01)

	// FPI1 - Functional parameter indicator 1 (1 byte)
	buf.WriteByte(0x00)

	// FPI2 - Functional parameter indicator 2 (1 byte)
	buf.WriteByte(0x00)

	// Main parameter (2 bytes) - position value
	binary.Write(buf, binary.BigEndian, mainParameter)

	// Functional parameters FP1-FP16 (16 x 2 bytes = 32 bytes) - set to ignore
	for i := 0; i < 16; i++ {
		if i < len(functionalParameters) {
			binary.Write(buf, binary.BigEndian, functionalParameters[i])
		} else {
			binary.Write(buf, binary.BigEndian, PositionIgnore)
		}
	}

	// Index array count (1 byte)
	buf.WriteByte(byte(len(nodeIDs)))

	// Node IDs (max 20, 1 byte each)
	for _, id := range nodeIDs {
		buf.WriteByte(id)
	}
	// Pad to 20 nodes
	for i := len(nodeIDs); i < MaxNodesPerRequest; i++ {
		buf.WriteByte(0)
	}

	// Priority level lock (1 byte) - 0 = no lock
	buf.WriteByte(0x00)

	// Priority level info (2 bytes)
	buf.Write([]byte{0x00, 0x00})

	// Lock time (1 byte) - 0 = no lock
	buf.WriteByte(0x00)

	return EncodeFrame(GW_COMMAND_SEND_REQ, buf.Bytes())
}

// BuildHouseStatusMonitorEnableRequest enables position change notifications
func BuildHouseStatusMonitorEnableRequest() []byte {
	return EncodeFrame(GW_HOUSE_STATUS_MONITOR_ENABLE_REQ, nil)
}

// ParsePasswordConfirm parses password confirmation response
// Status: 0 = successful, 1 = failed
func ParsePasswordConfirm(data []byte) (bool, error) {
	if len(data) < 1 {
		return false, ErrFrameTooShort
	}
	return data[0] == 0, nil
}

// ParseNodeInformation parses node information from GW_GET_ALL_NODES_INFORMATION_NTF
// Frame structure (124 bytes):
// - NodeID: 1 byte @ 0
// - Order: 2 bytes @ 1
// - Placement: 1 byte @ 3
// - Name: 64 bytes @ 4 (null-terminated UTF-8)
// - Velocity: 1 byte @ 68
// - NodeTypeSubType: 2 bytes @ 69
// - ProductGroup: 1 byte @ 71
// - ProductType: 1 byte @ 72
// - NodeVariation: 1 byte @ 73
// - PowerMode: 1 byte @ 74
// - BuildNumber: 1 byte @ 75
// - Serial: 8 bytes @ 76
// - State: 1 byte @ 84
// - CurrentPosition: 2 bytes @ 85
// - Target: 2 bytes @ 87
// - FP1-FP4: 8 bytes @ 89
// - RemainingTime: 2 bytes @ 97
// - TimeStamp: 4 bytes @ 99
// - NbrOfAlias: 1 byte @ 103
// - AliasArray: 20 bytes @ 104
func ParseNodeInformation(data []byte) (*Node, error) {
	if len(data) < 89 {
		return nil, fmt.Errorf("node information too short: %d bytes", len(data))
	}

	node := &Node{
		ID:            data[0],
		PositionValid: true,
	}

	// Name (64 bytes at offset 4, null-terminated UTF-8)
	nameEnd := 4
	for i := 4; i < 68 && data[i] != 0; i++ {
		nameEnd = i + 1
	}
	node.Name = string(data[4:nameEnd])

	// Velocity (1 byte at offset 68)
	node.Velocity = Velocity(data[68])

	// Node type (2 bytes at offset 69)
	node.NodeType = NodeType(binary.BigEndian.Uint16(data[69:71]))
	node.NodeTypeStr = node.NodeType.String()
	node.Inverted = node.NodeType.IsInvertedType()

	// State (1 byte at offset 84)
	node.State = NodeState(data[84])
	node.StateStr = node.State.String()

	// Current position (2 bytes at offset 85)
	node.CurrentPosition = binary.BigEndian.Uint16(data[85:87])
	node.PositionPercent = PositionToPercent(node.CurrentPosition)

	// Target position (2 bytes at offset 87)
	node.TargetPosition = binary.BigEndian.Uint16(data[87:89])
	node.TargetPercent = PositionToPercent(node.TargetPosition)

	return node, nil
}

// ParseNodeStatePositionChangedFull parses position change notification with all fields
// Frame structure:
// - NodeID: 1 byte @ 0
// - State: 1 byte @ 1
// - CurrentPosition: 2 bytes @ 2-3
// - Target: 2 bytes @ 4-5
func ParseNodeStatePositionChangedFull(data []byte) (nodeID uint8, state NodeState, position uint16, target uint16, err error) {
	if len(data) < 6 {
		return 0, 0, 0, 0, ErrFrameTooShort
	}

	nodeID = data[0]
	state = NodeState(data[1])
	position = binary.BigEndian.Uint16(data[2:4])
	target = binary.BigEndian.Uint16(data[4:6])

	return nodeID, state, position, target, nil
}

// ParseCommandSendConfirm parses command confirmation
// Frame structure: SessionID (2 bytes), Status (1 byte: 0 = rejected, 1 = accepted)
func ParseCommandSendConfirm(data []byte) (sessionID uint16, accepted bool, err error) {
	if len(data) < 3 {
		return 0, false, ErrFrameTooShort
	}

	sessionID = binary.BigEndian.Uint16(data[0:2])
	accepted = data[2] == 1

	return sessionID, accepted, nil
}

// ParseRunStatusNotification parses GW_COMMAND_RUN_STATUS_NTF
// Frame structure (13 bytes):
// - SessionID: 2 bytes @ 0
// - StatusID: 1 byte @ 2
// - Index (NodeID): 1 byte @ 3
// - NodeParameter: 1 byte @ 4
// - ParameterValue: 2 bytes @ 5
// - RunStatus: 1 byte @ 7
// - StatusReply: 1 byte @ 8
// - InformationCode: 4 bytes @ 9
func ParseRunStatusNotification(data []byte) (sessionID uint16, nodeID uint8, runStatus RunStatus, statusReply StatusReply, err error) {
	if len(data) < 13 {
		return 0, 0, 0, 0, ErrFrameTooShort
	}

	sessionID = binary.BigEndian.Uint16(data[0:2])
	nodeID = data[3]
	runStatus = RunStatus(data[7])
	statusReply = StatusReply(data[8])

	return sessionID, nodeID, runStatus, statusReply, nil
}

// ParseSessionID extracts the leading 2-byte session ID of a frame payload
// (GW_SESSION_FINISHED_NTF, GW_GET_LIMITATION_STATUS_CFM, ...).
func ParseSessionID(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, ErrFrameTooShort
	}
	return binary.BigEndian.Uint16(data[0:2]), nil
}

// BuildGetLimitationStatusRequest creates a request to get limitation status for nodes
// Frame structure:
// - SessionID: 2 bytes
// - IndexArrayCount: 1 byte
// - IndexArray: 20 bytes (node IDs, padded with 0)
// - ParameterID: 1 byte (0 = main parameter)
// - LimitationType: 1 byte (0 = min limitation, 1 = max limitation)
func BuildGetLimitationStatusRequest(sessionID uint16, nodeIDs []uint8, limitationType uint8) []byte {
	if len(nodeIDs) > MaxNodesPerRequest {
		nodeIDs = nodeIDs[:MaxNodesPerRequest]
	}

	buf := new(bytes.Buffer)

	// Session ID (2 bytes)
	binary.Write(buf, binary.BigEndian, sessionID)

	// Index array count (1 byte)
	buf.WriteByte(byte(len(nodeIDs)))

	// Node IDs (max 20, padded with 0)
	for _, id := range nodeIDs {
		buf.WriteByte(id)
	}
	for i := len(nodeIDs); i < MaxNodesPerRequest; i++ {
		buf.WriteByte(0)
	}

	// Parameter ID (1 byte) - 0 = main parameter
	buf.WriteByte(0x00)

	// Limitation type (1 byte) - 0 = min, 1 = max
	buf.WriteByte(limitationType)

	return EncodeFrame(GW_GET_LIMITATION_STATUS_REQ, buf.Bytes())
}

// ParseLimitationStatusNotification parses the limitation status notification
// Frame structure:
// - SessionID: 2 bytes @ 0
// - NodeID: 1 byte @ 2
// - ParameterID: 1 byte @ 3
// - MinValue: 2 bytes @ 4
// - MaxValue: 2 bytes @ 6
// - LimitationOriginator: 1 byte @ 8
// - LimitationTime: 1 byte @ 9
func ParseLimitationStatusNotification(data []byte) (*LimitationStatus, error) {
	if len(data) < 10 {
		return nil, ErrFrameTooShort
	}

	status := &LimitationStatus{
		SessionID:        binary.BigEndian.Uint16(data[0:2]),
		NodeID:           data[2],
		MinValue:         binary.BigEndian.Uint16(data[4:6]),
		MaxValue:         binary.BigEndian.Uint16(data[6:8]),
		LimitationOrigin: LimitationOriginator(data[8]),
		LimitationTime:   data[9],
	}

	return status, nil
}
