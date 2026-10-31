package api

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/configmgr"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
)

func TestMappingPersistenceFailuresReturn500(t *testing.T) {
	cfg := config.DefaultConfig()
	mgr := configmgr.New(cfg, filepath.Join(t.TempDir(), "missing", "config.yaml"), "", nil, zerolog.Nop())
	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop())
	ts := httptest.NewServer(NewServer(&cfg.Server, gw, zerolog.Nop(), mgr, "test").setupRoutes())
	defer ts.Close()
	resp, body := do(t, "POST", ts.URL+"/api/mappings", `{"node_id":1,"loxone_id":"test"}`, nil)
	if resp.StatusCode != 500 || len(mgr.GetConfig().Loxone.Mappings) != 0 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if strings.Contains(body, "missing") || strings.Contains(body, "details") {
		t.Fatal("internal filesystem path exposed")
	}
}

func TestParallelMappingRequestsAreAllPersisted(t *testing.T) {
	cfg := config.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.yaml")
	mgr := configmgr.New(cfg, path, "", nil, zerolog.Nop())
	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop())
	ts := httptest.NewServer(NewServer(&cfg.Server, gw, zerolog.Nop(), mgr, "test").setupRoutes())
	defer ts.Close()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			resp, body := do(t, "POST", ts.URL+"/api/mappings", fmt.Sprintf(`{"node_id":%d,"loxone_id":"m%d"}`, i, i), nil)
			if resp.StatusCode != 201 {
				t.Errorf("mapping %d: %d %s", i, resp.StatusCode, body)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Loxone.Mappings) != 12 {
		t.Fatalf("saved %d mappings", len(saved.Loxone.Mappings))
	}
}

func TestLoxoneUnknownAndInvalidStatus(t *testing.T) {
	ts, srv, gw, _ := newConnectedServer(t)
	srv.PushPositionChange(1, klf200.PositionUnknown)
	waitFor(t, "unknown position", func() bool { node, _ := gw.GetNode(1); return node.PositionPercent == -1 })
	if resp, body := do(t, "GET", ts.URL+"/loxone/node/1/position", "", nil); resp.StatusCode != 503 || body != "ERROR" {
		t.Fatalf("unknown position: %d %s", resp.StatusCode, body)
	}
	srv.DropConnections()
	waitFor(t, "disconnect", func() bool { return !gw.IsConnected() })
	for _, endpoint := range []string{"/loxone/sensors", "/loxone/sensors/rain", "/loxone/sensors/wind"} {
		if resp, body := do(t, "GET", ts.URL+endpoint, "", nil); resp.StatusCode != 503 || body != "ERROR" {
			t.Fatalf("stale sensor: %d %s", resp.StatusCode, body)
		}
	}
}
