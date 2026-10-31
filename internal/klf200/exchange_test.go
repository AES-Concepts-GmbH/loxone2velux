package klf200

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExchangeGateCancellationAndStopPriority(t *testing.T) {
	var gate exchangeGate
	if err := gate.acquire(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	order := make(chan string, 3)
	start := func(name string, stop bool, ctx context.Context) {
		go func() {
			if err := gate.acquire(ctx, stop); err != nil {
				order <- "cancelled"
				return
			}
			order <- name
			gate.release()
		}()
	}
	waitQueued := func(n int) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			gate.mu.Lock()
			count := len(gate.queue)
			gate.mu.Unlock()
			if count == n {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("exchange was not queued")
	}
	start("move", false, context.Background())
	waitQueued(1)
	start("expired", false, ctx)
	waitQueued(2)
	start("stop", true, context.Background())
	waitQueued(3)
	cancel()
	select {
	case got := <-order:
		if got != "cancelled" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation blocked")
	}
	gate.release()
	for _, want := range []string{"stop", "move"} {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("exchange blocked")
		}
	}
	if err := gate.acquire(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquisition = %v", err)
	}
}
