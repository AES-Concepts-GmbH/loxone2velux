package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
)

// memConfigManager is an in-memory ConfigManager for tests.
type memConfigManager struct {
	mu      sync.Mutex
	cfg     *config.Config
	gateway *gateway.Service // optional: receives UDP/mapping updates
}

func (m *memConfigManager) GetConfig() *config.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Clone()
}

func (m *memConfigManager) GetAPIToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Server.APIToken
}

func (m *memConfigManager) UpdateConfig(cfg *config.Config) error {
	return m.ModifyConfig(func(current *config.Config) error { *current = *cfg.Clone(); return nil })
}

func (m *memConfigManager) ModifyConfig(change func(*config.Config) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg.Clone()
	if err := change(cfg); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.cfg = cfg
	if m.gateway != nil {
		if err := m.gateway.GetUDPSender().Configure(cfg.Loxone.UDPFeedback); err != nil {
			return err
		}
		m.gateway.GetMappingManager().Load(cfg.Loxone.Mappings)
	}
	return nil
}

func (m *memConfigManager) GetConfigPath() string { return "" }

// httpGet returns the status code of a GET request.
func httpGet(url string) (int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func newTestServer(t *testing.T, token string) (*httptest.Server, *memConfigManager) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Server.APIToken = token
	cfg.KLF200.Password = "pw"
	cfg.KLF200.Host = "127.0.0.1"
	mgr := &memConfigManager{cfg: cfg}

	// Gateway without a KLF-200: never started, so all commands report "not connected".
	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop())
	srv := NewServer(&cfg.Server, gw, zerolog.Nop(), mgr, "test")
	ts := httptest.NewServer(srv.setupRoutes())
	t.Cleanup(ts.Close)
	return ts, mgr
}

func do(t *testing.T, method, url, body string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp, sb.String()
}

func TestTokenAuth(t *testing.T) {
	const token = "0123456789abcdef0123"
	ts, mgr := newTestServer(t, token)

	// health is public
	if resp, _ := do(t, "GET", ts.URL+"/health", "", nil); resp.StatusCode != 200 {
		t.Errorf("health status = %d", resp.StatusCode)
	}
	if _, body := do(t, "GET", ts.URL+"/api/auth/status", "", nil); !strings.Contains(body, `"required":true`) {
		t.Errorf("auth status = %s", body)
	}

	// no token -> 401
	if resp, _ := do(t, "GET", ts.URL+"/api/nodes", "", nil); resp.StatusCode != 401 {
		t.Errorf("without token status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/loxone/sensors/rain", "", nil); resp.StatusCode != 401 {
		t.Errorf("loxone without token status = %d", resp.StatusCode)
	}

	// bearer header and query parameter
	if resp, _ := do(t, "GET", ts.URL+"/api/nodes", "", map[string]string{"Authorization": "Bearer " + token}); resp.StatusCode != 200 {
		t.Errorf("bearer status = %d", resp.StatusCode)
	}
	if resp, body := do(t, "GET", ts.URL+"/loxone/sensors?token="+token, "", nil); resp.StatusCode != 503 || body != "ERROR" {
		t.Errorf("query token status = %d body = %q", resp.StatusCode, body)
	}

	// token change takes effect without restart
	cfg := mgr.GetConfig()
	cfg.Server.APIToken = ""
	if err := mgr.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/nodes", "", nil); resp.StatusCode != 200 {
		t.Errorf("after removing token status = %d", resp.StatusCode)
	}
}

func TestConfigDoesNotLeakSecrets(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, body := do(t, "GET", ts.URL+"/api/config", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(body, "pw") || strings.Contains(body, `"password"`) || strings.Contains(body, `"api_token"`) {
		t.Errorf("secrets leaked: %s", body)
	}
	var got ConfigResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if !got.KLF200.PasswordSet || got.Server.APITokenSet {
		t.Errorf("flags: %+v %+v", got.KLF200, got.Server)
	}
}

func TestUpdateConfigValidation(t *testing.T) {
	ts, mgr := newTestServer(t, "")

	resp, _ := do(t, "POST", ts.URL+"/api/config", `{"server":{"api_token":"short"}}`, nil)
	if resp.StatusCode != 400 {
		t.Errorf("short token status = %d, want 400", resp.StatusCode)
	}

	resp, _ = do(t, "POST", ts.URL+"/api/config", `{"logging":{"level":"debug"},"klf200":{"password":""}}`, nil)
	if resp.StatusCode != 200 {
		t.Errorf("valid update status = %d", resp.StatusCode)
	}
	cfg := mgr.GetConfig()
	if cfg.Logging.Level != "debug" || cfg.KLF200.Password != "pw" {
		t.Errorf("update not applied / empty password overwrote: %+v", cfg)
	}
}

func TestMappingCRUDAndValidation(t *testing.T) {
	ts, _ := newTestServer(t, "")

	resp, body := do(t, "POST", ts.URL+"/api/mappings", `{"name":"a","node_id":1,"loxone_id":"win_a"}`, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d body = %s", resp.StatusCode, body)
	}
	var created config.NodeMapping
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !created.Enabled {
		t.Errorf("created = %+v", created)
	}

	// duplicate loxone_id -> 400
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"node_id":2,"loxone_id":"win_a"}`, nil); resp.StatusCode != 400 {
		t.Errorf("duplicate loxone_id status = %d", resp.StatusCode)
	}
	// same node enabled twice -> 400
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"node_id":1,"loxone_id":"win_b"}`, nil); resp.StatusCode != 400 {
		t.Errorf("duplicate node status = %d", resp.StatusCode)
	}
	// same node disabled -> ok
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"node_id":1,"loxone_id":"win_b","enabled":false}`, nil); resp.StatusCode != 201 {
		t.Errorf("disabled duplicate node status = %d", resp.StatusCode)
	}

	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/"+created.ID, `{"name":"renamed","node_id":1,"loxone_id":"win_a"}`, nil); resp.StatusCode != 200 {
		t.Errorf("update status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "PUT", ts.URL+"/api/mappings/nope", `{"loxone_id":"x"}`, nil); resp.StatusCode != 404 {
		t.Errorf("update unknown status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "DELETE", ts.URL+"/api/mappings/"+created.ID, "", nil); resp.StatusCode != 200 {
		t.Errorf("delete status = %d", resp.StatusCode)
	}
	_, body = do(t, "GET", ts.URL+"/api/mappings", "", nil)
	if !strings.Contains(body, `"count":1`) {
		t.Errorf("after delete: %s", body)
	}
}

func TestLoxoneEndpointsWithoutGateway(t *testing.T) {
	ts, _ := newTestServer(t, "")

	if resp, body := do(t, "GET", ts.URL+"/loxone/node/1/open", "", nil); resp.StatusCode != 503 || body != "ERROR" {
		t.Errorf("open without KLF-200: status = %d body = %q", resp.StatusCode, body)
	}
	if resp, body := do(t, "GET", ts.URL+"/loxone/node/1/set/150", "", nil); resp.StatusCode != 400 || body != "ERROR" {
		t.Errorf("set out of range: status = %d body = %q", resp.StatusCode, body)
	}
	if resp, _ := do(t, "GET", ts.URL+"/loxone/node/300/open", "", nil); resp.StatusCode != 400 {
		t.Errorf("node id out of range status = %d", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/loxone/node/1/position", "", nil); resp.StatusCode != 404 {
		t.Errorf("unknown node position status = %d", resp.StatusCode)
	}
	if resp, body := do(t, "POST", ts.URL+"/api/nodes/1/position", `{"position":42}`, nil); resp.StatusCode != 503 || !strings.Contains(body, "not connected") {
		t.Errorf("api position without KLF-200: status = %d body = %s", resp.StatusCode, body)
	}
}

func TestIngressBasePathIsEscaped(t *testing.T) {
	ts, _ := newTestServer(t, "")
	srv := &Server{logger: zerolog.Nop()}
	dir := t.TempDir()
	index := dir + "/index.html"
	if err := os.WriteFile(index, []byte("<html><head><title>x</title></head></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := srv.indexHandler(index)
	req := httptest.NewRequest("GET", ts.URL+"/", nil)
	req.Header.Set("X-Ingress-Path", `/api/hassio_ingress/abc"><script>alert(1)</script>`)
	rr := httptest.NewRecorder()
	h(rr, req)
	body := rr.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("ingress path not escaped: %s", body)
	}
	if !strings.Contains(body, `<base href="/api/hassio_ingress/abc&#34;&gt;`) {
		t.Errorf("base tag missing or wrong: %s", body)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	ts, _ := newTestServer(t, "")

	// A body above the limit is rejected before it is fully buffered.
	huge := `{"name":"` + strings.Repeat("a", maxRequestBodyBytes+1) + `","loxone_id":"x"}`
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", huge, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized body status = %d, want 400", resp.StatusCode)
	}

	// A normal body still works.
	if resp, _ := do(t, "POST", ts.URL+"/api/mappings", `{"name":"n","loxone_id":"x"}`, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("normal body status = %d, want 201", resp.StatusCode)
	}
}

func TestConfigRoutesWithoutConfigManager(t *testing.T) {
	cfg := config.DefaultConfig()
	gw := gateway.NewService(&cfg.KLF200, &cfg.Loxone, zerolog.Nop())
	srv := NewServer(&cfg.Server, gw, zerolog.Nop(), nil, "test")
	ts := httptest.NewServer(srv.setupRoutes())
	t.Cleanup(ts.Close)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/config"},
		{"POST", "/api/config"},
		{"GET", "/api/mappings"},
		{"POST", "/api/mappings"},
		{"GET", "/api/loxone/config"},
	} {
		if resp, _ := do(t, tc.method, ts.URL+tc.path, "{}", nil); resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s status = %d, want 503", tc.method, tc.path, resp.StatusCode)
		}
	}

	// Routes that do not need the config manager keep working.
	if resp, _ := do(t, "GET", ts.URL+"/health", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d", resp.StatusCode)
	}
}
