package tabs

import (
	"rocket4cli/internal/config"
	"rocket4cli/internal/proxy"
)

type configSaver interface {
	SaveConfig(*config.Config) error
}

func PersistConfig(cfg *config.Config, slotMgr proxy.SlotService) error {
	if saver, ok := slotMgr.(configSaver); ok {
		return saver.SaveConfig(cfg)
	}
	return cfg.Save()
}

func persistConfig(cfg *config.Config, slotMgr proxy.SlotService) error {
	return PersistConfig(cfg, slotMgr)
}
