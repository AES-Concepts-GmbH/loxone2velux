package configmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stefanbeyeler/loxone2velux/internal/config"
)

func TestConcurrentModificationsSurviveReload(t *testing.T) {
	m, path, _ := newManager(t, nil)
	start := make(chan struct{})
	errors := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errors <- m.ModifyConfig(func(c *config.Config) error {
				id := fmt.Sprintf("m%d", i)
				c.Loxone.Mappings = append(c.Loxone.Mappings, config.NodeMapping{ID: id, LoxoneID: id})
				return nil
			})
		}(i)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Loxone.Mappings) != 12 || len(m.GetConfig().Loxone.Mappings) != 12 {
		t.Fatal("concurrent changes were lost")
	}
}

func TestManagedConfigurationPersistsOnlyLoxone(t *testing.T) {
	m, mainPath, sidePath := newManager(t, nil)
	m.SetManagedConfig()
	if err := m.GetConfig().Save(mainPath); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(mainPath)
	err := m.ModifyConfig(func(c *config.Config) error { c.Server.APIToken = "0123456789abcdef"; return nil })
	if !config.IsValidationError(err) {
		t.Fatalf("managed token change = %v", err)
	}
	err = m.ModifyConfig(func(c *config.Config) error {
		c.Loxone.Mappings = []config.NodeMapping{{ID: "m", LoxoneID: "m"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	lc, err := config.LoadLoxoneConfig(sidePath)
	if err != nil || len(lc.Mappings) != 1 {
		t.Fatalf("sidecar: %+v %v", lc, err)
	}
	after, _ := os.ReadFile(mainPath)
	if string(before) != string(after) {
		t.Fatal("HA-owned config changed")
	}

	// An unwritable sidecar must leave both the runtime and HA file untouched.
	m.loxonePath = filepath.Join(t.TempDir(), "missing", "loxone.yaml")
	err = m.ModifyConfig(func(c *config.Config) error { c.Loxone.Mappings = nil; return nil })
	if err == nil || len(m.GetConfig().Loxone.Mappings) != 1 {
		t.Fatal("failed sidecar update was applied")
	}
}
