package klf200

import (
	"context"
	"sync"
)

// exchangeGate serializes responses, allows cancellation while queued, and
// schedules waiting stop commands before ordinary exchanges. An exchange
// already on the wire must finish before the next one can consume responses.
type exchangeGate struct {
	mu    sync.Mutex
	busy  bool
	queue []*exchangeWaiter
}

type exchangeWaiter struct {
	ready chan struct{}
	stop  bool
}

func (g *exchangeGate) acquire(ctx context.Context, stop bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w := &exchangeWaiter{ready: make(chan struct{}), stop: stop}
	g.mu.Lock()
	if !g.busy {
		g.busy = true
		g.mu.Unlock()
		return nil
	}
	g.queue = append(g.queue, w)
	g.mu.Unlock()
	select {
	case <-w.ready:
		if err := ctx.Err(); err != nil {
			g.release()
			return err
		}
		return nil
	case <-ctx.Done():
		g.mu.Lock()
		for i, queued := range g.queue {
			if queued == w {
				g.queue = append(g.queue[:i], g.queue[i+1:]...)
				g.mu.Unlock()
				return ctx.Err()
			}
		}
		g.mu.Unlock()
		// Ownership was transferred concurrently with cancellation.
		<-w.ready
		g.release()
		return ctx.Err()
	}
}

func (g *exchangeGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.queue) == 0 {
		g.busy = false
		return
	}
	i := 0
	for j, w := range g.queue {
		if w.stop {
			i = j
			break
		}
	}
	w := g.queue[i]
	g.queue = append(g.queue[:i], g.queue[i+1:]...)
	close(w.ready)
}
