package api

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
)

// Server represents the HTTP API server
type Server struct {
	cfg       *config.ServerConfig
	gateway   *gateway.Service
	logger    zerolog.Logger
	server    *http.Server
	configMgr ConfigManager
	version   string
}

// NewServer creates a new API server
func NewServer(cfg *config.ServerConfig, gw *gateway.Service, logger zerolog.Logger, configMgr ConfigManager, version string) *Server {
	cfgCopy := *cfg
	return &Server{
		cfg:       &cfgCopy,
		gateway:   gw,
		logger:    logger.With().Str("component", "api").Logger(),
		configMgr: configMgr,
		version:   version,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	router := s.setupRoutes()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	s.server = &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       s.cfg.ReadTimeout,
		ReadHeaderTimeout: s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
	}

	s.logger.Info().Str("addr", addr).Msg("Starting HTTP server")

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("Shutting down HTTP server")
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// apiToken returns the currently configured API token ("" = no auth).
func (s *Server) apiToken() string {
	if s.configMgr != nil {
		return s.configMgr.GetAPIToken()
	}
	return s.cfg.APIToken
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	// Collapse double slashes in URL paths (HA Ingress can produce them
	// from ingress_entry: / appended to the proxy base path)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			for strings.Contains(req.URL.Path, "//") {
				req.URL.Path = strings.ReplaceAll(req.URL.Path, "//", "/")
			}
			if req.URL.RawPath != "" {
				for strings.Contains(req.URL.RawPath, "//") {
					req.URL.RawPath = strings.ReplaceAll(req.URL.RawPath, "//", "/")
				}
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(NewLoggingMiddleware(s.logger))
	r.Use(chimiddleware.Recoverer)
	// The request context deadline must not exceed the server's write timeout,
	// otherwise the connection is cut before the handler can answer.
	requestTimeout := s.cfg.WriteTimeout
	if requestTimeout <= 0 {
		requestTimeout = 15 * time.Second
	}
	r.Use(chimiddleware.Timeout(requestTimeout))

	// Handlers
	h := NewHandlers(s.gateway, s.logger, s.configMgr, s.version)
	auth := NewTokenAuthMiddleware(s.apiToken, s.logger)

	// Public routes (no auth required)
	r.Get("/health", h.Health)

	// Auth status endpoint - tells frontend if auth is required
	r.Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"required": s.apiToken() != ""})
	})

	// API routes - protected only if token is configured
	r.Route("/api", func(r chi.Router) {
		r.Use(auth)
		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", h.ListNodes)
			r.Get("/{nodeID}", h.GetNode)
			r.Post("/{nodeID}/position", h.SetPosition)
			r.Post("/{nodeID}/open", h.OpenNode)
			r.Post("/{nodeID}/close", h.CloseNode)
			r.Post("/{nodeID}/stop", h.StopNode)
		})
		r.Route("/sensors", func(r chi.Router) {
			r.Get("/", h.GetSensorStatus)
			r.Post("/refresh", h.RefreshSensorStatus)
		})
		// Mapping endpoints
		r.Route("/mappings", func(r chi.Router) {
			r.Get("/", h.ListMappings)
			r.Post("/", h.CreateMapping)
			r.Put("/{mappingID}", h.UpdateMapping)
			r.Delete("/{mappingID}", h.DeleteMapping)
		})
		// Loxone integration config
		r.Route("/loxone", func(r chi.Router) {
			r.Get("/config", h.GetLoxoneConfig)
			r.Put("/config/udp", h.UpdateLoxoneUDPConfig)
			r.Post("/config/udp/test", h.TestUDP)
		})
		// Configuration endpoints
		r.Get("/config", h.GetConfig)
		r.Post("/config", h.UpdateConfig)
		r.Post("/reconnect", h.Reconnect)
	})

	// Loxone-friendly endpoints - protected only if token is configured
	// Token via query param: /loxone/node/1/open?token=YOUR_TOKEN
	r.Route("/loxone", func(r chi.Router) {
		r.Use(auth)
		r.Get("/node/{nodeID}/position", h.LoxoneGetPosition)
		r.Get("/node/{nodeID}/set/{position}", h.LoxoneSetPosition)
		r.Get("/node/{nodeID}/open", h.LoxoneOpen)
		r.Get("/node/{nodeID}/close", h.LoxoneClose)
		r.Get("/node/{nodeID}/stop", h.LoxoneStop)
		r.Get("/sensors", h.LoxoneSensorStatus)
		r.Get("/sensors/rain", h.LoxoneRainStatus)
		r.Get("/sensors/wind", h.LoxoneWindStatus)
	})

	// Static files for web frontend
	staticDir := "./web/dist"
	if _, err := os.Stat(staticDir); err == nil {
		s.logger.Info().Str("dir", staticDir).Msg("Serving static files")
		indexPath := filepath.Join(staticDir, "index.html")
		serveIndex := s.indexHandler(indexPath)

		// Root path: serve index.html with HA Ingress base path injection.
		r.Get("/", serveIndex)
		fileServer(r, http.Dir(staticDir), serveIndex)
	} else {
		s.logger.Debug().Msg("No static files directory found, skipping frontend")
	}

	return r
}

// indexHandler serves index.html. HA Ingress sends an X-Ingress-Path header;
// we inject a <base> tag so that all relative URLs (API calls, assets)
// resolve correctly behind the proxy. The header value is HTML-escaped
// before it is written into the page.
func (s *Server) indexHandler(indexPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		htmlBytes, err := os.ReadFile(indexPath)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		page := string(htmlBytes)
		if ingressPath := req.Header.Get("X-Ingress-Path"); ingressPath != "" && strings.HasPrefix(ingressPath, "/") {
			if !strings.HasSuffix(ingressPath, "/") {
				ingressPath += "/"
			}
			page = strings.Replace(page, "<head>", `<head><base href="`+html.EscapeString(ingressPath)+`" />`, 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(page))
	}
}

// fileServer serves the static frontend files from root. Unknown paths fall
// back to the SPA index page.
func fileServer(r chi.Router, root http.FileSystem, serveIndex http.HandlerFunc) {
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		fsPath := req.URL.Path

		f, err := root.Open(fsPath)
		if err != nil {
			// If file not found, serve index.html for SPA routing
			serveIndex(w, req)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			serveIndex(w, req)
			return
		}

		http.ServeContent(w, req, stat.Name(), stat.ModTime(), f)
	})
}
