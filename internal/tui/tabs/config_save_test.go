package tabs

import (
	"testing"

	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

type recordingConfigSaver struct {
	proxy.SlotService
	called bool
}

func (r *recordingConfigSaver) SaveConfig(cfg *config.Config) error {
	r.called = true
	return nil
}

func TestPersistConfigUsesSlotServiceWhenAvailable(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())

	saver := &recordingConfigSaver{}
	if err := PersistConfig(config.DefaultConfig(), saver); err != nil {
		t.Fatalf("PersistConfig: %v", err)
	}
	if !saver.called {
		t.Fatal("expected config to be pushed through SlotService.SaveConfig")
	}
}
