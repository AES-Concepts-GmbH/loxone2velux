package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
	"github.com/stefanbeyeler/loxone2velux/internal/gateway"
	"github.com/stefanbeyeler/loxone2velux/internal/klf200"
)

// ConfigManager interface for configuration operations
type ConfigManager interface {
	GetConfig() *config.Config
	GetAPIToken() string
	UpdateConfig(cfg *config.Config) error
	GetConfigPath() string
}

// Handlers holds all HTTP handlers
type Handlers struct {
	gateway   *gateway.Service
	logger    zerolog.Logger
	configMgr ConfigManager
	version   string
}

// NewHandlers creates new handlers
func NewHandlers(gw *gateway.Service, logger zerolog.Logger, configMgr ConfigManager, version string) *Handlers {
	return &Handlers{
		gateway:   gw,
		logger:    logger.With().Str("component", "handlers").Logger(),
		configMgr: configMgr,
		version:   version,
	}
}

// Response types
type HealthResponse struct {
	Status    string `json:"status"`
	Connected bool   `json:"connected"`
	NodeCount int    `json:"node_count"`
	Version   string `json:"version"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}

type PositionRequest struct {
	Position float64 `json:"position"`
}

type CommandResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	NodeID  uint8  `json:"node_id"`
}

type NodesResponse struct {
	Nodes []*klf200.Node `json:"nodes"`
	Count int            `json:"count"`
}

// Health returns the health status
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:    "ok",
		Connected: h.gateway.IsConnected(),
		NodeCount: h.gateway.GetNodeCount(),
		Version:   h.version,
	}

	if !resp.Connected {
		resp.Status = "degraded"
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListNodes returns all nodes
func (h *Handlers) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.gateway.GetNodes()

	resp := NodesResponse{
		Nodes: nodes,
		Count: len(nodes),
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetNode returns a single node
func (h *Handlers) GetNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node ID", err.Error())
		return
	}

	node, ok := h.gateway.GetNode(nodeID)
	if !ok {
		writeError(w, http.StatusNotFound, "Node not found", "")
		return
	}

	writeJSON(w, http.StatusOK, node)
}

// commandStatus maps a gateway command error to an HTTP status code.
func commandStatus(err error) int {
	if errors.Is(err, gateway.ErrNotConnected) || errors.Is(err, gateway.ErrNotConfigured) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

// SetPosition sets the position of a node
func (h *Handlers) SetPosition(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node ID", err.Error())
		return
	}

	var req PositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Position < 0 || req.Position > 100 || math.IsNaN(req.Position) {
		writeError(w, http.StatusBadRequest, "Position must be between 0 and 100", "")
		return
	}

	if err := h.gateway.SetPosition(r.Context(), nodeID, req.Position); err != nil {
		writeError(w, commandStatus(err), "Failed to set position", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, CommandResponse{
		Success: true,
		Message: "Position command sent",
		NodeID:  nodeID,
	})
}

// OpenNode fully opens a node
func (h *Handlers) OpenNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node ID", err.Error())
		return
	}

	if err := h.gateway.Open(r.Context(), nodeID); err != nil {
		writeError(w, commandStatus(err), "Failed to open node", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, CommandResponse{
		Success: true,
		Message: "Open command sent",
		NodeID:  nodeID,
	})
}

// CloseNode fully closes a node
func (h *Handlers) CloseNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node ID", err.Error())
		return
	}

	if err := h.gateway.Close(r.Context(), nodeID); err != nil {
		writeError(w, commandStatus(err), "Failed to close node", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, CommandResponse{
		Success: true,
		Message: "Close command sent",
		NodeID:  nodeID,
	})
}

// StopNode stops a node's movement
func (h *Handlers) StopNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node ID", err.Error())
		return
	}

	if err := h.gateway.StopNode(r.Context(), nodeID); err != nil {
		writeError(w, commandStatus(err), "Failed to stop node", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, CommandResponse{
		Success: true,
		Message: "Stop command sent",
		NodeID:  nodeID,
	})
}

// Loxone-friendly endpoints (GET requests with URL parameters, plain-text responses)

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// loxoneCommand runs a node command and answers with OK/ERROR.
func (h *Handlers) loxoneCommand(w http.ResponseWriter, r *http.Request, action string,
	run func(nodeID uint8) error) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writePlain(w, http.StatusBadRequest, "ERROR")
		return
	}

	if err := run(nodeID); err != nil {
		h.logger.Error().Err(err).Uint8("node", nodeID).Str("action", action).Msg("Loxone command failed")
		writePlain(w, commandStatus(err), "ERROR")
		return
	}

	writePlain(w, http.StatusOK, "OK")
}

// LoxoneSetPosition handles Loxone position requests via URL
func (h *Handlers) LoxoneSetPosition(w http.ResponseWriter, r *http.Request) {
	positionStr := chi.URLParam(r, "position")
	position, err := strconv.ParseFloat(positionStr, 64)
	if err != nil || math.IsNaN(position) || position < 0 || position > 100 {
		writePlain(w, http.StatusBadRequest, "ERROR")
		return
	}

	h.loxoneCommand(w, r, "set", func(nodeID uint8) error {
		return h.gateway.SetPosition(r.Context(), nodeID, position)
	})
}

// LoxoneOpen handles Loxone open requests
func (h *Handlers) LoxoneOpen(w http.ResponseWriter, r *http.Request) {
	h.loxoneCommand(w, r, "open", func(nodeID uint8) error {
		return h.gateway.Open(r.Context(), nodeID)
	})
}

// LoxoneClose handles Loxone close requests
func (h *Handlers) LoxoneClose(w http.ResponseWriter, r *http.Request) {
	h.loxoneCommand(w, r, "close", func(nodeID uint8) error {
		return h.gateway.Close(r.Context(), nodeID)
	})
}

// LoxoneStop handles Loxone stop requests
func (h *Handlers) LoxoneStop(w http.ResponseWriter, r *http.Request) {
	h.loxoneCommand(w, r, "stop", func(nodeID uint8) error {
		return h.gateway.StopNode(r.Context(), nodeID)
	})
}

// LoxoneGetPosition returns the current position of a node as a plain number (0-100)
func (h *Handlers) LoxoneGetPosition(w http.ResponseWriter, r *http.Request) {
	nodeID, err := parseNodeID(r)
	if err != nil {
		writePlain(w, http.StatusBadRequest, "ERROR")
		return
	}

	node, ok := h.gateway.GetNode(nodeID)
	if !ok {
		writePlain(w, http.StatusNotFound, "ERROR")
		return
	}

	writePlain(w, http.StatusOK, strconv.Itoa(int(math.Round(node.PositionPercent))))
}

// Sensor endpoints

// GetSensorStatus returns the current sensor status (rain, wind, etc.)
func (h *Handlers) GetSensorStatus(w http.ResponseWriter, r *http.Request) {
	status := h.gateway.GetSensorStatus()
	writeJSON(w, http.StatusOK, status)
}

// RefreshSensorStatus triggers a refresh of sensor data from the KLF-200
func (h *Handlers) RefreshSensorStatus(w http.ResponseWriter, r *http.Request) {
	if err := h.gateway.RefreshSensorStatus(r.Context()); err != nil {
		writeError(w, commandStatus(err), "Failed to refresh sensor status", err.Error())
		return
	}

	status := h.gateway.GetSensorStatus()
	writeJSON(w, http.StatusOK, status)
}

// Loxone-friendly sensor endpoints (return simple 0/1 values)

func boolToDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// LoxoneSensorStatus returns all sensor values in Loxone-friendly format
func (h *Handlers) LoxoneSensorStatus(w http.ResponseWriter, r *http.Request) {
	status := h.gateway.GetSensorStatus()
	// Format: rain;wind (e.g., "0;0" or "1;0")
	writePlain(w, http.StatusOK, boolToDigit(status.RainDetected)+";"+boolToDigit(status.WindDetected))
}

// LoxoneRainStatus returns just the rain sensor value (0 or 1)
func (h *Handlers) LoxoneRainStatus(w http.ResponseWriter, r *http.Request) {
	writePlain(w, http.StatusOK, boolToDigit(h.gateway.GetSensorStatus().RainDetected))
}

// LoxoneWindStatus returns just the wind sensor value (0 or 1)
func (h *Handlers) LoxoneWindStatus(w http.ResponseWriter, r *http.Request) {
	writePlain(w, http.StatusOK, boolToDigit(h.gateway.GetSensorStatus().WindDetected))
}

// Configuration endpoints

// ConfigResponse is the JSON structure returned by GET /api/config. Secrets
// are never returned; *_set flags tell whether a value is configured.
type ConfigResponse struct {
	KLF200  ConfigKLF200  `json:"klf200"`
	Server  ConfigServer  `json:"server"`
	Loxone  ConfigLoxone  `json:"loxone"`
	Logging ConfigLogging `json:"logging"`
}

type ConfigLoxone struct {
	UDPFeedback config.UDPFeedbackConfig `json:"udp_feedback"`
	Mappings    []config.NodeMapping     `json:"mappings"`
}

type ConfigKLF200 struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	PasswordSet       bool   `json:"password_set"`
	ReconnectInterval string `json:"reconnect_interval"`
	RefreshInterval   string `json:"refresh_interval"`
}

type ConfigServer struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	APITokenSet bool   `json:"api_token_set"`
}

type ConfigLogging struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// ConfigUpdateRequest is the JSON structure accepted by POST /api/config.
// Omitted fields are left unchanged. An explicit empty api_token disables
// authentication.
type ConfigUpdateRequest struct {
	KLF200 struct {
		Host     *string `json:"host"`
		Port     *int    `json:"port"`
		Password *string `json:"password"`
	} `json:"klf200"`
	Server struct {
		APIToken *string `json:"api_token"`
	} `json:"server"`
	Logging struct {
		Level *string `json:"level"`
	} `json:"logging"`
}

func (h *Handlers) configResponse(cfg *config.Config) ConfigResponse {
	return ConfigResponse{
		KLF200: ConfigKLF200{
			Host:              cfg.KLF200.Host,
			Port:              cfg.KLF200.Port,
			PasswordSet:       cfg.KLF200.Password != "",
			ReconnectInterval: cfg.KLF200.ReconnectInterval.String(),
			RefreshInterval:   cfg.KLF200.RefreshInterval.String(),
		},
		Server: ConfigServer{
			Host:        cfg.Server.Host,
			Port:        cfg.Server.Port,
			APITokenSet: cfg.Server.APIToken != "",
		},
		Loxone: ConfigLoxone{
			UDPFeedback: cfg.Loxone.UDPFeedback,
			Mappings:    cfg.Loxone.Mappings,
		},
		Logging: ConfigLogging{
			Level:  cfg.Logging.Level,
			Format: cfg.Logging.Format,
		},
	}
}

// RequireConfigManager answers 503 for routes that need the configuration
// manager when none is available.
func (h *Handlers) RequireConfigManager(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.configMgr == nil {
			writeError(w, http.StatusServiceUnavailable, "Configuration manager not available", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetConfig returns the current configuration (without secrets)
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.configResponse(h.configMgr.GetConfig()))
}

// UpdateConfig updates the configuration
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Get current config and update it
	cfg := h.configMgr.GetConfig()

	if req.KLF200.Host != nil {
		cfg.KLF200.Host = *req.KLF200.Host
	}
	if req.KLF200.Port != nil {
		cfg.KLF200.Port = *req.KLF200.Port
	}
	if req.KLF200.Password != nil && *req.KLF200.Password != "" {
		cfg.KLF200.Password = *req.KLF200.Password
	}
	if req.Server.APIToken != nil {
		cfg.Server.APIToken = *req.Server.APIToken
	}
	if req.Logging.Level != nil {
		cfg.Logging.Level = *req.Logging.Level
	}

	if err := h.saveConfig(w, cfg, "Failed to save configuration"); err != nil {
		return
	}

	h.logger.Info().Msg("Configuration updated")

	writeJSON(w, http.StatusOK, h.configResponse(cfg))
}

// saveConfig persists cfg and writes an error response on failure
// (400 for validation errors, 500 otherwise).
func (h *Handlers) saveConfig(w http.ResponseWriter, cfg *config.Config, message string) error {
	err := h.configMgr.UpdateConfig(cfg)
	if err == nil {
		return nil
	}
	status := http.StatusInternalServerError
	if config.IsValidationError(err) {
		status = http.StatusBadRequest
	}
	writeError(w, status, message, err.Error())
	return err
}

// Reconnect triggers a reconnection to the KLF-200
func (h *Handlers) Reconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.gateway.Reconnect(r.Context()); err != nil {
		writeJSON(w, commandStatus(err), map[string]interface{}{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Reconnected",
	})
}

// Helper functions

func parseNodeID(r *http.Request) (uint8, error) {
	nodeIDStr := chi.URLParam(r, "nodeID")
	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 8)
	if err != nil {
		return 0, err
	}
	return uint8(nodeID), nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message, details string) {
	writeJSON(w, status, ErrorResponse{
		Error:   message,
		Code:    status,
		Details: details,
	})
}

// generateUUID generates a random UUID v4
func generateUUID() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

// Mapping CRUD endpoints

// mappingRequest is the JSON body for creating/updating a mapping. "enabled"
// defaults to true when omitted.
type mappingRequest struct {
	Name     string `json:"name"`
	NodeID   uint8  `json:"node_id"`
	LoxoneID string `json:"loxone_id"`
	Enabled  *bool  `json:"enabled"`
}

func (m *mappingRequest) enabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// ListMappings returns all node-to-Loxone mappings
func (h *Handlers) ListMappings(w http.ResponseWriter, r *http.Request) {
	mappings := h.configMgr.GetConfig().Loxone.Mappings
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
		"count":    len(mappings),
	})
}

// CreateMapping creates a new node mapping
func (h *Handlers) CreateMapping(w http.ResponseWriter, r *http.Request) {
	var req mappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.LoxoneID == "" {
		writeError(w, http.StatusBadRequest, "loxone_id is required", "")
		return
	}

	id, err := generateUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate mapping ID", err.Error())
		return
	}
	mapping := config.NodeMapping{
		ID:       id,
		Name:     req.Name,
		NodeID:   req.NodeID,
		LoxoneID: req.LoxoneID,
		Enabled:  req.enabled(),
	}

	cfg := h.configMgr.GetConfig()
	cfg.Loxone.Mappings = append(cfg.Loxone.Mappings, mapping)
	if err := h.saveConfig(w, cfg, "Failed to save mapping"); err != nil {
		return
	}

	writeJSON(w, http.StatusCreated, mapping)
}

// UpdateMapping updates an existing mapping
func (h *Handlers) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingID")

	var req mappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.LoxoneID == "" {
		writeError(w, http.StatusBadRequest, "loxone_id is required", "")
		return
	}

	cfg := h.configMgr.GetConfig()
	var updated *config.NodeMapping
	for i := range cfg.Loxone.Mappings {
		if cfg.Loxone.Mappings[i].ID == mappingID {
			cfg.Loxone.Mappings[i] = config.NodeMapping{
				ID:       mappingID,
				Name:     req.Name,
				NodeID:   req.NodeID,
				LoxoneID: req.LoxoneID,
				Enabled:  req.enabled(),
			}
			updated = &cfg.Loxone.Mappings[i]
			break
		}
	}

	if updated == nil {
		writeError(w, http.StatusNotFound, "Mapping not found", "")
		return
	}

	if err := h.saveConfig(w, cfg, "Failed to save mapping"); err != nil {
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// DeleteMapping deletes a mapping
func (h *Handlers) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingID")

	cfg := h.configMgr.GetConfig()
	newMappings := make([]config.NodeMapping, 0, len(cfg.Loxone.Mappings))
	found := false

	for _, m := range cfg.Loxone.Mappings {
		if m.ID == mappingID {
			found = true
			continue
		}
		newMappings = append(newMappings, m)
	}

	if !found {
		writeError(w, http.StatusNotFound, "Mapping not found", "")
		return
	}

	cfg.Loxone.Mappings = newMappings
	if err := h.saveConfig(w, cfg, "Failed to delete mapping"); err != nil {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Loxone config endpoints

// GetLoxoneConfig returns the Loxone-specific configuration
func (h *Handlers) GetLoxoneConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.configMgr.GetConfig()
	writeJSON(w, http.StatusOK, ConfigLoxone{
		UDPFeedback: cfg.Loxone.UDPFeedback,
		Mappings:    cfg.Loxone.Mappings,
	})
}

// UpdateLoxoneUDPConfig updates UDP feedback settings
func (h *Handlers) UpdateLoxoneUDPConfig(w http.ResponseWriter, r *http.Request) {
	var req config.UDPFeedbackConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	cfg := h.configMgr.GetConfig()
	cfg.Loxone.UDPFeedback = req

	if err := h.saveConfig(w, cfg, "Failed to save config"); err != nil {
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"udp_feedback": req,
	})
}

// TestUDP sends a test UDP message
func (h *Handlers) TestUDP(w http.ResponseWriter, r *http.Request) {
	udpSender := h.gateway.GetUDPSender()
	if udpSender == nil || !udpSender.IsEnabled() {
		writeError(w, http.StatusBadRequest, "UDP feedback is not enabled", "")
		return
	}

	udpSender.Send("test", "ping", 1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "test sent"})
}
