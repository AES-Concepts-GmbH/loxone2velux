package klf200

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// payloadFrame builds a raw (unescaped) protocol frame for a command and
// payload, exactly as the KLF-200 sends it before SLIP encoding.
func payloadFrame(cmd CommandID, payload []byte) []byte {
	raw := []byte{ProtocolID, byte(len(payload) + 3)}
	raw = binary.BigEndian.AppendUint16(raw, uint16(cmd))
	raw = append(raw, payload...)
	crc := byte(0)
	for _, b := range raw {
		crc ^= b
	}
	return append(raw, crc)
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Payload contains both SLIP special bytes to exercise escaping.
	payload := []byte{0x01, SlipEnd, 0x02, SlipEsc, 0x03}
	frame := EncodeFrame(GW_COMMAND_SEND_REQ, payload)

	if frame[0] != SlipEnd || frame[len(frame)-1] != SlipEnd {
		t.Fatalf("frame not SLIP delimited: % x", frame)
	}
	if bytes.Contains(frame[1:len(frame)-1], []byte{SlipEnd}) {
		t.Fatalf("unescaped END byte inside frame: % x", frame)
	}

	decoded, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if decoded.Command != GW_COMMAND_SEND_REQ {
		t.Errorf("command = 0x%04x, want 0x%04x", uint16(decoded.Command), uint16(GW_COMMAND_SEND_REQ))
	}
	if !bytes.Equal(decoded.Data, payload) {
		t.Errorf("data = % x, want % x", decoded.Data, payload)
	}
}

func TestDecodeFrameChecksum(t *testing.T) {
	raw := payloadFrame(GW_PASSWORD_ENTER_CFM, []byte{0x00})
	raw[len(raw)-1] ^= 0xFF
	if _, err := DecodeFrame(slipEncode(raw)); err != ErrChecksumMismatch {
		t.Errorf("err = %v, want ErrChecksumMismatch", err)
	}
}

func TestDecodeFrameLengthMismatch(t *testing.T) {
	raw := payloadFrame(GW_PASSWORD_ENTER_CFM, []byte{0x00})
	raw[1] = 9 // wrong declared length
	// fix checksum so only the length check fails
	crc := byte(0)
	for _, b := range raw[:len(raw)-1] {
		crc ^= b
	}
	raw[len(raw)-1] = crc
	if _, err := DecodeFrame(slipEncode(raw)); err == nil {
		t.Error("expected error for wrong declared length")
	}
}

func TestDecodeFrameTooShort(t *testing.T) {
	if _, err := DecodeFrame([]byte{SlipEnd, 0x00, SlipEnd}); err == nil {
		t.Error("expected error for short frame")
	}
}

func TestBuildPasswordEnterRequest(t *testing.T) {
	frame, err := DecodeFrame(BuildPasswordEnterRequest("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if frame.Command != GW_PASSWORD_ENTER_REQ {
		t.Errorf("command = 0x%04x", uint16(frame.Command))
	}
	if len(frame.Data) != 32 {
		t.Fatalf("password payload length = %d, want 32", len(frame.Data))
	}
	if string(frame.Data[:6]) != "secret" || frame.Data[6] != 0 {
		t.Errorf("payload = % x", frame.Data)
	}
}

func TestBuildCommandSendRequestLayout(t *testing.T) {
	frame, err := DecodeFrame(BuildCommandSendRequest(0x1234, 1, PriorityUserLevel2, []uint8{7}, PositionCurrent, nil))
	if err != nil {
		t.Fatal(err)
	}
	d := frame.Data
	if len(d) != 66 {
		t.Fatalf("payload length = %d, want 66", len(d))
	}
	if binary.BigEndian.Uint16(d[0:2]) != 0x1234 {
		t.Errorf("session id = 0x%04x", binary.BigEndian.Uint16(d[0:2]))
	}
	if d[2] != 1 || d[3] != byte(PriorityUserLevel2) || d[4] != 0x01 {
		t.Errorf("originator/priority/active = % x", d[2:5])
	}
	if mp := binary.BigEndian.Uint16(d[7:9]); mp != PositionCurrent {
		t.Errorf("main parameter = 0x%04x, want 0x%04x", mp, PositionCurrent)
	}
	for i := 0; i < 16; i++ {
		if fp := binary.BigEndian.Uint16(d[9+2*i : 11+2*i]); fp != PositionIgnore {
			t.Errorf("FP%d = 0x%04x, want ignore", i+1, fp)
		}
	}
	if d[41] != 1 || d[42] != 7 {
		t.Errorf("index array = % x", d[41:62])
	}
}

func TestSpecialPositionValues(t *testing.T) {
	// Cross-checked against the KLF-200 specification / pyvlx Parameter class.
	if PositionTarget != 0xD100 || PositionCurrent != 0xD200 || PositionDefault != 0xD300 || PositionIgnore != 0xD400 {
		t.Errorf("special position values do not match the specification")
	}
	if GW_GET_LIMITATION_STATUS_REQ != 0x0312 || GW_LIMITATION_STATUS_NTF != 0x0314 || GW_GET_STATE_REQ != 0x000C {
		t.Errorf("command ids do not match the specification")
	}
	if LimitationOriginatorRain != 2 || LimitationOriginatorWind != 9 || LimitationOriginatorUser != 1 {
		t.Errorf("limitation originator values do not match the specification")
	}
}

func TestParseCommandSendConfirm(t *testing.T) {
	sid, accepted, err := ParseCommandSendConfirm([]byte{0x00, 0x05, 0x01})
	if err != nil || sid != 5 || !accepted {
		t.Errorf("accepted: sid=%d accepted=%v err=%v", sid, accepted, err)
	}
	_, accepted, _ = ParseCommandSendConfirm([]byte{0x00, 0x05, 0x00})
	if accepted {
		t.Error("status 0 must be reported as rejected")
	}
	if _, _, err := ParseCommandSendConfirm([]byte{0x00}); err != ErrFrameTooShort {
		t.Errorf("short frame err = %v", err)
	}
}

func TestParseRunStatusNotification(t *testing.T) {
	// SessionID=0x0102, StatusID=1, Index(node)=9, NodeParameter=0,
	// ParameterValue=0xC800, RunStatus=2 (active), StatusReply=0xE3 (rain), InfoCode=0
	payload := []byte{0x01, 0x02, 0x01, 0x09, 0x00, 0xC8, 0x00, 0x02, 0xE3, 0, 0, 0, 0}
	sid, node, run, reply, err := ParseRunStatusNotification(payload)
	if err != nil {
		t.Fatal(err)
	}
	if sid != 0x0102 || node != 9 {
		t.Errorf("sid=%d node=%d", sid, node)
	}
	if run != RunStatusExecutionActive {
		t.Errorf("runStatus = %d, want %d (byte 7)", run, RunStatusExecutionActive)
	}
	if reply != StatusReplyLimitationByRain {
		t.Errorf("statusReply = 0x%02x, want 0x%02x (byte 8)", uint8(reply), uint8(StatusReplyLimitationByRain))
	}
}

func TestParseLimitationStatusNotification(t *testing.T) {
	payload := []byte{0x00, 0x07, 0x03, 0x00, 0x00, 0x00, 0xC8, 0x00, 0x02, 0xFD}
	st, err := ParseLimitationStatusNotification(payload)
	if err != nil {
		t.Fatal(err)
	}
	if st.SessionID != 7 || st.NodeID != 3 || st.MinValue != 0 || st.MaxValue != 0xC800 {
		t.Errorf("unexpected status: %+v", st)
	}
	if st.LimitationOrigin != LimitationOriginatorRain || !st.Active() {
		t.Errorf("origin=%v active=%v", st.LimitationOrigin, st.Active())
	}

	none := &LimitationStatus{MinValue: PositionUnknown, MaxValue: PositionUnknown, LimitationOrigin: LimitationOriginatorRain}
	if none.Active() {
		t.Error("unknown bounds must mean no active limitation")
	}
}

func TestParseNodeInformation(t *testing.T) {
	payload := make([]byte, 124)
	payload[0] = 4
	copy(payload[4:], "Dachfenster")
	payload[68] = byte(VelocitySilent)
	binary.BigEndian.PutUint16(payload[69:71], uint16(NodeTypeWindowOpenerWithRainSensor))
	payload[84] = byte(NodeStateDone)
	binary.BigEndian.PutUint16(payload[85:87], 0x6400) // 50%
	binary.BigEndian.PutUint16(payload[87:89], 0xC800) // 100%

	node, err := ParseNodeInformation(payload)
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != 4 || node.Name != "Dachfenster" || node.Velocity != VelocitySilent {
		t.Errorf("node = %+v", node)
	}
	if node.NodeTypeStr != "Window Opener (rain sensor)" {
		t.Errorf("type str = %q", node.NodeTypeStr)
	}
	if node.PositionPercent != 50 || node.TargetPercent != 100 || !node.PositionValid {
		t.Errorf("positions: %v / %v", node.PositionPercent, node.TargetPercent)
	}
	if node.StateStr != "Done" {
		t.Errorf("state = %q", node.StateStr)
	}
}

func TestNodeTypeString(t *testing.T) {
	cases := map[NodeType]string{
		NodeTypeWindowOpener:      "Window Opener",
		0x0102:                    "Window Opener", // unknown sub type falls back to base type
		NodeTypeRollerShutter:     "Roller Shutter",
		NodeTypeDualRollerShutter: "Dual Roller Shutter",
		NodeTypeLight:             "Light",
		NodeTypeHorizontalAwning:  "Horizontal Awning",
		NodeTypeSwingingShutters:  "Swinging Shutter",
		0x7FC0:                    "Unknown (0x7FC0)",
	}
	for nt, want := range cases {
		if got := nt.String(); got != want {
			t.Errorf("NodeType(0x%04X).String() = %q, want %q", uint16(nt), got, want)
		}
	}
}

func TestPercentConversion(t *testing.T) {
	if PercentToPosition(0) != PositionMin || PercentToPosition(100) != PositionMax {
		t.Error("endpoints")
	}
	if PercentToPosition(-5) != PositionMin || PercentToPosition(150) != PositionMax {
		t.Error("clamping")
	}
	if PositionToPercent(0xC800) != 100 || PositionToPercent(0) != 0 {
		t.Error("raw endpoints")
	}
	if PositionToPercent(PositionUnknown) != 100 {
		t.Error("out-of-range raw values clamp to 100")
	}
	for _, p := range []float64{0, 1, 25, 50, 99, 100} {
		if got := PositionToPercent(PercentToPosition(p)); got < p-0.01 || got > p+0.01 {
			t.Errorf("round trip %v -> %v", p, got)
		}
	}
}
