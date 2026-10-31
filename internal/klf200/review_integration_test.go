package klf200_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200/klf200test"
)

func TestExpiredQueuedCommandNeverReachesGateway(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	srv.SetSilent(true)
	blockCtx, release := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.GetState(blockCtx) }()
	waitFor(t, "blocked keep-alive", func() bool { return srv.StateRequests() > 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c.Close(ctx, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired command = %v", err)
	}
	release()
	<-done
	srv.SetSilent(false)
	if err := c.Stop(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	commands := srv.Commands()
	if len(commands) != 1 || commands[0].MainParameter != klf200.PositionCurrent {
		t.Fatalf("expired command reached wire: %+v", commands)
	}
}

func TestIncompleteSensorRefreshNeverPublishesFreshState(t *testing.T) {
	for _, fault := range []string{"missing", "duplicate", "session"} {
		t.Run(fault, func(t *testing.T) {
			srv := klf200test.NewServer(t, testPassword, testNodes()...)
			c := connect(t, srv)
			if err := c.RefreshSensorStatus(context.Background(), []uint8{0, 1}); err != nil {
				t.Fatal(err)
			}
			before := c.GetSensorStatus()
			srv.SetLimitation(0, klf200.LimitationOriginatorRain)
			srv.SetLimitationFault(fault)
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			if err := c.RefreshSensorStatus(ctx, []uint8{0, 1}); err == nil {
				t.Fatal("partial refresh accepted")
			}
			after := c.GetSensorStatus()
			if after.Valid || !after.LastUpdate.Equal(before.LastUpdate) || after.RainDetected {
				t.Fatalf("partial state published: %+v", after)
			}
		})
	}
}

func TestSlowPerNodeRepliesCompleteWithinPerFrameBudget(t *testing.T) {
	frameTimeout := 150 * time.Millisecond
	defer klf200.SetLimitationFrameTimeout(frameTimeout)()
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	// A full chunk answered one node at a time takes far longer than a single
	// frame budget; every individual frame still arrives well within it.
	srv.SetLimitationFault("slow")
	nodeIDs := make([]uint8, klf200.MaxNodesPerRequest)
	for i := range nodeIDs {
		nodeIDs[i] = uint8(i)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := c.RefreshSensorStatus(ctx, nodeIDs); err != nil {
		t.Fatalf("slow but complete refresh failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed <= frameTimeout {
		t.Fatalf("simulator did not exceed one frame budget (%v)", elapsed)
	}
	if !c.GetSensorStatus().Valid {
		t.Fatal("complete refresh not published")
	}
}

func TestOversizedSLIPStreamRecoversAtNextFrame(t *testing.T) {
	srv := klf200test.NewServer(t, testPassword, testNodes()...)
	c := connect(t, srv)
	srv.PushRaw(append([]byte{klf200.SlipEnd}, bytes.Repeat([]byte{1}, 64*1024)...))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.GetState(ctx); err != nil {
		t.Fatalf("valid frame after oversized stream: %v", err)
	}
}
