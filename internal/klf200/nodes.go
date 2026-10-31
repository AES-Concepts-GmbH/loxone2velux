package klf200

import (
	"sync"
	"time"
)

// NodeManager manages the node cache
type NodeManager struct {
	nodes map[uint8]*Node
	mu    sync.RWMutex
}

// NewNodeManager creates a new node manager
func NewNodeManager() *NodeManager {
	return &NodeManager{
		nodes: make(map[uint8]*Node),
	}
}

// SetNodes replaces all nodes
func (m *NodeManager) SetNodes(nodes []*Node) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nodes = make(map[uint8]*Node)
	for _, node := range nodes {
		m.nodes[node.ID] = node
	}
}

// GetNode returns a node by ID
func (m *NodeManager) GetNode(id uint8) (*Node, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, ok := m.nodes[id]
	if !ok {
		return nil, false
	}

	// Return a copy
	nodeCopy := *node
	return &nodeCopy, true
}

// GetAllNodes returns all nodes
func (m *NodeManager) GetAllNodes() []*Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]*Node, 0, len(m.nodes))
	for _, node := range m.nodes {
		nodeCopy := *node
		nodes = append(nodes, &nodeCopy)
	}
	return nodes
}

// UpdateNode updates a node's position
func (m *NodeManager) UpdateNode(update *Node) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, ok := m.nodes[update.ID]; ok {
		// Only overwrite position data when the update actually carries it.
		// State-only notifications (run status) leave PositionValid false and
		// must not reset the cached position to zero.
		if update.PositionValid {
			node.CurrentPosition = update.CurrentPosition
			node.PositionPercent = update.PositionPercent
			node.TargetPosition = update.TargetPosition
			node.TargetPercent = update.TargetPercent
			node.LastUpdate = time.Now()
		}
		node.State = update.State
		if update.StateStr != "" {
			node.StateStr = update.StateStr
		}
	}
}

// NodeCount returns the number of nodes
func (m *NodeManager) NodeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}
