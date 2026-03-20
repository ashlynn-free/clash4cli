package config

import (
	"sort"
	"strings"
)

// MaxSlotConfigs matches the runtime slot limit.
const MaxSlotConfigs = 10

// normalize trims persisted slot settings, removes invalid entries, and sorts by slot ID.
func (c *Config) normalizeSlots() {
	if c == nil {
		return
	}

	normalized := make([]SlotConfig, 0, len(c.Slots))
	seen := make(map[int]struct{}, len(c.Slots))
	for _, slot := range c.Slots {
		if slot.ID < 0 || slot.ID >= MaxSlotConfigs {
			continue
		}
		if _, exists := seen[slot.ID]; exists {
			continue
		}
		slot.Upstream = slot.Upstream.normalized()
		if slot.Upstream.isEmpty() {
			continue
		}
		normalized = append(normalized, slot)
		seen[slot.ID] = struct{}{}
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})
	c.Slots = normalized
}

// GetSlotUpstream returns the stored upstream config for a slot, if any.
func (c *Config) GetSlotUpstream(slotID int) (UpstreamProxyConfig, bool) {
	if c == nil {
		return UpstreamProxyConfig{}, false
	}
	for _, slot := range c.Slots {
		if slot.ID == slotID {
			return slot.Upstream.normalized(), true
		}
	}
	return UpstreamProxyConfig{}, false
}

// EffectiveUpstreamForSlot returns a usable upstream config for runtime chaining.
func (c *Config) EffectiveUpstreamForSlot(slotID int) (UpstreamProxyConfig, bool) {
	upstream, ok := c.GetSlotUpstream(slotID)
	if !ok {
		return UpstreamProxyConfig{}, false
	}
	if !upstream.Enabled {
		return UpstreamProxyConfig{}, false
	}
	if upstream.Protocol == "" || upstream.Server == "" || upstream.Port <= 0 || upstream.Port >= 65536 {
		return UpstreamProxyConfig{}, false
	}
	return upstream, true
}

// SetSlotUpstream writes or clears the persisted upstream config for a slot.
func (c *Config) SetSlotUpstream(slotID int, upstream UpstreamProxyConfig) {
	if c == nil {
		return
	}

	upstream = upstream.normalized()
	for i := range c.Slots {
		if c.Slots[i].ID != slotID {
			continue
		}
		if upstream.isEmpty() {
			c.Slots = append(c.Slots[:i], c.Slots[i+1:]...)
		} else {
			c.Slots[i].Upstream = upstream
		}
		c.normalizeSlots()
		return
	}

	if upstream.isEmpty() {
		return
	}
	c.Slots = append(c.Slots, SlotConfig{
		ID:       slotID,
		Upstream: upstream,
	})
	c.normalizeSlots()
}

func (u UpstreamProxyConfig) normalized() UpstreamProxyConfig {
	u.Protocol = strings.ToLower(strings.TrimSpace(u.Protocol))
	u.Server = strings.TrimSpace(u.Server)
	u.Username = strings.TrimSpace(u.Username)
	u.Password = strings.TrimSpace(u.Password)
	return u
}

func (u UpstreamProxyConfig) isEmpty() bool {
	return !u.Enabled &&
		strings.TrimSpace(u.Protocol) == "" &&
		strings.TrimSpace(u.Server) == "" &&
		u.Port == 0 &&
		strings.TrimSpace(u.Username) == "" &&
		strings.TrimSpace(u.Password) == ""
}
