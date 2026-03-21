package config

import (
	"fmt"
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
		// Keep the slot if it has an upstream registry reference OR a non-empty legacy inline config.
		if slot.UpstreamID == "" && slot.Upstream.isEmpty() {
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

// GetSlotUpstream returns the stored legacy inline upstream config for a slot, if any.
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
// It first checks for a registry reference (UpstreamID), then falls back to legacy inline config.
func (c *Config) EffectiveUpstreamForSlot(slotID int) (UpstreamProxyConfig, bool) {
	// New path: resolve via upstream proxy registry.
	if ref, ok := c.GetSlotUpstreamRef(slotID); ok && ref != "" {
		entry, found := c.GetUpstreamProxy(ref)
		if !found {
			return UpstreamProxyConfig{}, false
		}
		upstream := UpstreamProxyConfig{
			Enabled:  true,
			Protocol: entry.Protocol,
			Server:   entry.Server,
			Port:     entry.Port,
			Username: entry.Username,
			Password: entry.Password,
		}
		upstream = upstream.normalized()
		if upstream.Protocol == "" || upstream.Server == "" || upstream.Port <= 0 || upstream.Port >= 65536 {
			return UpstreamProxyConfig{}, false
		}
		return upstream, true
	}

	// Legacy path: inline upstream config.
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

// GetSlotUpstreamRef returns the upstream registry ID for a slot, if any.
func (c *Config) GetSlotUpstreamRef(slotID int) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, slot := range c.Slots {
		if slot.ID == slotID {
			return slot.UpstreamID, true
		}
	}
	return "", false
}

// SetSlotUpstreamRef sets a slot to reference an upstream proxy by registry ID.
// Pass "" to clear the upstream reference.
func (c *Config) SetSlotUpstreamRef(slotID int, upstreamID string) {
	if c == nil {
		return
	}

	for i := range c.Slots {
		if c.Slots[i].ID != slotID {
			continue
		}
		c.Slots[i].UpstreamID = upstreamID
		// Clear legacy inline config when setting a registry reference.
		c.Slots[i].Upstream = UpstreamProxyConfig{}
		c.normalizeSlots()
		return
	}

	if upstreamID == "" {
		return
	}
	c.Slots = append(c.Slots, SlotConfig{
		ID:         slotID,
		UpstreamID: upstreamID,
	})
	c.normalizeSlots()
}

// SetSlotUpstream writes or clears the persisted upstream config for a slot.
// Deprecated: use SetSlotUpstreamRef with the upstream proxy registry instead.
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

// migrateInlineUpstreams converts legacy per-slot inline upstream configs into
// upstream proxy registry entries. Called once during Load().
func (c *Config) migrateInlineUpstreams() {
	if c == nil {
		return
	}
	for i := range c.Slots {
		slot := &c.Slots[i]
		if slot.UpstreamID != "" || slot.Upstream.isEmpty() {
			continue
		}
		entry := UpstreamProxyEntry{
			Label:    fmt.Sprintf("Slot %d upstream", slot.ID),
			Protocol: slot.Upstream.Protocol,
			Server:   slot.Upstream.Server,
			Port:     slot.Upstream.Port,
			Username: slot.Upstream.Username,
			Password: slot.Upstream.Password,
		}
		id := c.AddUpstreamProxy(entry)
		if slot.Upstream.Enabled {
			slot.UpstreamID = id
		}
		slot.Upstream = UpstreamProxyConfig{}
	}
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
