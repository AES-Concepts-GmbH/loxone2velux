package loxone

import (
	"sync"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
)

// MappingManager handles KLF-200 Node ID to Loxone ID mappings
type MappingManager struct {
	byNodeID map[uint8]*config.NodeMapping
	byID     map[string]*config.NodeMapping
	mu       sync.RWMutex
}

// NewMappingManager creates a new mapping manager
func NewMappingManager() *MappingManager {
	return &MappingManager{
		byNodeID: make(map[uint8]*config.NodeMapping),
		byID:     make(map[string]*config.NodeMapping),
	}
}

// Load initializes mappings from a config list
func (m *MappingManager) Load(mappings []config.NodeMapping) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byNodeID = make(map[uint8]*config.NodeMapping)
	m.byID = make(map[string]*config.NodeMapping)

	for i := range mappings {
		// Copy so the manager does not alias the caller's slice.
		mapping := new(config.NodeMapping)
		*mapping = mappings[i]
		m.byID[mapping.ID] = mapping
		if mapping.Enabled {
			m.byNodeID[mapping.NodeID] = mapping
		}
	}
}

// GetByNodeID returns a mapping by KLF-200 node ID
func (m *MappingManager) GetByNodeID(nodeID uint8) *config.NodeMapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byNodeID[nodeID]
}

// GetByID returns a mapping by its UUID
func (m *MappingManager) GetByID(id string) *config.NodeMapping {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byID[id]
}

// GetAll returns all mappings (including disabled ones)
func (m *MappingManager) GetAll() []config.NodeMapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]config.NodeMapping, 0, len(m.byID))
	for _, mapping := range m.byID {
		result = append(result, *mapping)
	}
	return result
}

// Add adds a new mapping
func (m *MappingManager) Add(mapping *config.NodeMapping) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byID[mapping.ID] = mapping
	if mapping.Enabled {
		m.byNodeID[mapping.NodeID] = mapping
	}
}

// Remove removes a mapping by UUID
func (m *MappingManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mapping, ok := m.byID[id]; ok {
		delete(m.byID, id)
		if existing, exists := m.byNodeID[mapping.NodeID]; exists && existing.ID == id {
			delete(m.byNodeID, mapping.NodeID)
		}
	}
}
