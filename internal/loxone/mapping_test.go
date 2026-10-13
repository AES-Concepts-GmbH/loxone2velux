package loxone

import (
	"testing"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
)

func TestMappingManagerLoadAndLookup(t *testing.T) {
	m := NewMappingManager()
	m.Load([]config.NodeMapping{
		{ID: "a", NodeID: 1, LoxoneID: "one", Enabled: true},
		{ID: "b", NodeID: 2, LoxoneID: "two", Enabled: false},
	})

	if got := m.GetByNodeID(1); got == nil || got.LoxoneID != "one" {
		t.Errorf("GetByNodeID(1) = %+v", got)
	}
	if got := m.GetByNodeID(2); got != nil {
		t.Errorf("disabled mapping returned for node 2: %+v", got)
	}
	if got := m.GetByID("b"); got == nil || got.NodeID != 2 {
		t.Errorf("GetByID(b) = %+v", got)
	}
	if all := m.GetAll(); len(all) != 2 {
		t.Errorf("GetAll len = %d", len(all))
	}

	// Load replaces everything
	m.Load(nil)
	if m.GetByID("a") != nil || len(m.GetAll()) != 0 {
		t.Error("Load(nil) did not clear mappings")
	}
}

func TestMappingManagerAddRemove(t *testing.T) {
	m := NewMappingManager()
	m.Add(&config.NodeMapping{ID: "a", NodeID: 5, LoxoneID: "x", Enabled: true})
	m.Add(&config.NodeMapping{ID: "b", NodeID: 5, LoxoneID: "y", Enabled: true})

	// Removing "a" must not drop the node index that now belongs to "b"
	m.Remove("a")
	if got := m.GetByNodeID(5); got == nil || got.ID != "b" {
		t.Errorf("GetByNodeID(5) after Remove(a) = %+v", got)
	}
	m.Remove("b")
	if m.GetByNodeID(5) != nil || m.GetByID("b") != nil {
		t.Error("Remove(b) left entries behind")
	}
	m.Remove("does-not-exist") // must not panic
}

func TestMappingManagerLoadCopiesInput(t *testing.T) {
	src := []config.NodeMapping{{ID: "a", NodeID: 1, LoxoneID: "one", Enabled: true}}
	m := NewMappingManager()
	m.Load(src)
	src[0].LoxoneID = "changed"
	if got := m.GetByNodeID(1); got.LoxoneID != "one" {
		t.Errorf("manager aliases caller's slice: %q", got.LoxoneID)
	}
}
