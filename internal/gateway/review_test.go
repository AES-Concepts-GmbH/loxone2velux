package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200/klf200test"
)

func TestUnchangedStatesAreResentOnRefresh(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	port, msgs := udpListener(t)
	s := startService(t, srv, &config.LoxoneConfig{
		UDPFeedback: config.UDPFeedbackConfig{Enabled: true, IP: "127.0.0.1", Port: port},
		Mappings:    []config.NodeMapping{{ID: "m", NodeID: 1, LoxoneID: "m", Enabled: true}},
	})
	collect(t, msgs, "m/position:100")
	for len(msgs) > 0 {
		<-msgs
	}
	if err := s.refreshNodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	collect(t, msgs, "m/position:100")
	if err := s.RefreshSensorStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	collect(t, msgs, "m/rain:0")
}

func TestStalenessDoesNotTreatStateUpdatesAsPositionMeasurements(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	s := startService(t, srv, nil)
	if n, _ := s.GetNode(1); n.Stale {
		t.Fatal("fresh node is stale")
	}
	s.nodes.SetNodes([]*klf200.Node{{ID: 1, PositionPercent: 100, LastUpdate: time.Now().Add(-time.Hour)}})
	s.nodes.UpdateNode(&klf200.Node{ID: 1, State: klf200.NodeStateDone})
	if n, _ := s.GetNode(1); !n.Stale {
		t.Fatal("state-only update refreshed position age")
	}
	s.client.Disconnect()
	if s.GetSensorStatus().Valid {
		t.Fatal("disconnected sensor still valid")
	}
}
