package klf200

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func newTestClient() *Client {
	return NewClient(ClientConfig{Host: "127.0.0.1", Port: 51200, Password: "x", Logger: zerolog.Nop()})
}

func TestSensorStatusAggregation(t *testing.T) {
	c := newTestClient()
	var updates []SensorStatus
	c.SetSensorUpdateCallback(func(s SensorStatus) { updates = append(updates, s) })

	// Rain on node 1, wind on node 2
	c.updateSensorStatus(&LimitationStatus{NodeID: 1, MinValue: 0, MaxValue: 0xC800, LimitationOrigin: LimitationOriginatorRain})
	c.updateSensorStatus(&LimitationStatus{NodeID: 2, MinValue: 0, MaxValue: 0xC800, LimitationOrigin: LimitationOriginatorWind})
	st := c.GetSensorStatus()
	if !st.RainDetected || !st.WindDetected {
		t.Fatalf("expected rain and wind, got %+v", st)
	}

	// A node without limitation must not clear the other node's rain flag
	c.updateSensorStatus(&LimitationStatus{NodeID: 3, MinValue: PositionUnknown, MaxValue: PositionUnknown})
	if st := c.GetSensorStatus(); !st.RainDetected || !st.WindDetected {
		t.Fatalf("unrelated node cleared sensors: %+v", st)
	}

	// A user limitation (originator 1) is not rain
	c.updateSensorStatus(&LimitationStatus{NodeID: 1, MinValue: 0, MaxValue: 0xC800, LimitationOrigin: LimitationOriginatorUser})
	if st := c.GetSensorStatus(); st.RainDetected || !st.WindDetected {
		t.Fatalf("user limitation handled as rain: %+v", st)
	}

	// Clearing node 2 clears wind
	c.updateSensorStatus(&LimitationStatus{NodeID: 2, MinValue: PositionUnknown, MaxValue: PositionUnknown, LimitationOrigin: LimitationOriginatorWind})
	if st := c.GetSensorStatus(); st.RainDetected || st.WindDetected {
		t.Fatalf("expected all clear, got %+v", st)
	}

	// Callback fires only on changes: rain on, wind on, rain off, wind off = 4
	if len(updates) != 4 {
		t.Errorf("sensor callback fired %d times, want 4", len(updates))
	}
}

func TestRunStatusSetsAndClearsLimitation(t *testing.T) {
	c := newTestClient()
	var nodeUpdates []*Node
	c.SetNodeUpdateCallback(func(n *Node) { nodeUpdates = append(nodeUpdates, n) })

	rain := []byte{0, 1, 1, 5, 0, 0, 0, byte(RunStatusExecutionFailed), byte(StatusReplyLimitationByRain), 0, 0, 0, 0}
	c.handleAsyncFrame(&Frame{Command: GW_COMMAND_RUN_STATUS_NTF, Data: rain})
	if !c.GetSensorStatus().RainDetected {
		t.Fatal("rain limitation from run status not detected")
	}
	if len(nodeUpdates) != 1 || nodeUpdates[0].ID != 5 || nodeUpdates[0].State != NodeStateErrorWhileExecution || nodeUpdates[0].PositionValid {
		t.Fatalf("unexpected node update: %+v", nodeUpdates)
	}

	ok := []byte{0, 2, 1, 5, 0, 0, 0, byte(RunStatusExecutionCompleted), byte(StatusReplyCommandCompletedOk), 0, 0, 0, 0}
	c.handleAsyncFrame(&Frame{Command: GW_COMMAND_RUN_STATUS_NTF, Data: ok})
	if c.GetSensorStatus().RainDetected {
		t.Fatal("completed command did not clear the node's rain limitation")
	}
}

func TestVerifyVeluxCertificate(t *testing.T) {
	// The embedded CA itself is signed by the Velux CA (self-signed root), so
	// presenting it as the leaf must verify, even though it has expired.
	if err := verifyVeluxCertificate([][]byte{veluxCA.Raw}, nil); err != nil {
		t.Errorf("Velux CA chain rejected: %v", err)
	}

	// A self-signed certificate from an unrelated issuer must be rejected.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "not-velux"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyVeluxCertificate([][]byte{der}, nil); err == nil {
		t.Error("certificate not signed by the Velux CA was accepted")
	}

	if err := verifyVeluxCertificate(nil, nil); err == nil {
		t.Error("empty chain was accepted")
	}
}
