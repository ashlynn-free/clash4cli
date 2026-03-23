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
		slot.SubID = strings.TrimSpace(slot.SubID)
		slot.NodeName = strings.TrimSpace(slot.NodeName)
		slot.NodeServer = strings.TrimSpace(slot.NodeServer)
		slot.NodeType = strings.TrimSpace(slot.NodeType)
		slot.StartMode = normalizeSlotStartMode(slot.StartMode)
		slot.PortMode = normalizeSlotPortMode(slot.PortMode)
		if slot.NodePort < 0 || slot.NodePort >= 65536 {
			slot.NodePort = 0
		}
		if slot.ManualMixedPort <= 0 || slot.ManualMixedPort >= 65536 {
			slot.ManualMixedPort = 0
		}
		slot.Upstream = slot.Upstream.normalized()
		if !slot.hasPersistedSettings() {
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

func (s SlotConfig) hasPersistedSettings() bool {
	hasNode := s.SubID != "" || s.NodeName != "" || s.NodeServer != "" || s.NodePort > 0 || s.NodeType != ""
	hasMode := s.StartMode != ""
	hasPort := s.PortMode == "manual" || s.ManualMixedPort > 0
	hasUpstream := s.UpstreamID != "" || !s.Upstream.isEmpty()
	hasCachedTest := s.LastLatency != 0 || s.LastRegion != ""
	return hasNode || hasMode || hasPort || hasUpstream || hasCachedTest
}

func normalizeSlotStartMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "rule":
		return "rule"
	case "global":
		return "global"
	default:
		return ""
	}
}

func normalizeSlotPortMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "manual":
		return "manual"
	case "auto":
		return "auto"
	default:
		return ""
	}
}

func (c *Config) slotIndex(slotID int) int {
	if c == nil {
		return -1
	}
	for i := range c.Slots {
		if c.Slots[i].ID == slotID {
			return i
		}
	}
	return -1
}

func (c *Config) ensureSlotConfig(slotID int) *SlotConfig {
	if c == nil || slotID < 0 || slotID >= MaxSlotConfigs {
		return nil
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		return &c.Slots[idx]
	}
	c.Slots = append(c.Slots, SlotConfig{ID: slotID})
	sort.Slice(c.Slots, func(i, j int) bool {
		return c.Slots[i].ID < c.Slots[j].ID
	})
	return &c.Slots[c.slotIndex(slotID)]
}

// GetSlotConfig returns the normalized config for a slot, if any persisted state exists.
func (c *Config) GetSlotConfig(slotID int) (SlotConfig, bool) {
	if c == nil {
		return SlotConfig{}, false
	}
	idx := c.slotIndex(slotID)
	if idx < 0 {
		return SlotConfig{}, false
	}
	slot := c.Slots[idx]
	slot.StartMode = normalizeSlotStartMode(slot.StartMode)
	slot.PortMode = normalizeSlotPortMode(slot.PortMode)
	if !slot.hasPersistedSettings() {
		return SlotConfig{}, false
	}
	return slot, true
}

// GetSlotNodeRef returns the persisted node selection for a slot.
func (c *Config) GetSlotNodeRef(slotID int) (SlotNodeRef, bool) {
	slot, ok := c.GetSlotConfig(slotID)
	if !ok {
		return SlotNodeRef{}, false
	}
	ref := SlotNodeRef{
		SubID:      slot.SubID,
		NodeName:   slot.NodeName,
		NodeServer: slot.NodeServer,
		NodePort:   slot.NodePort,
		NodeType:   slot.NodeType,
	}
	if strings.TrimSpace(ref.SubID) == "" && strings.TrimSpace(ref.NodeName) == "" && strings.TrimSpace(ref.NodeServer) == "" && ref.NodePort == 0 {
		return SlotNodeRef{}, false
	}
	return ref, true
}

// SetSlotNodeRef sets or clears the persisted node selection for a slot.
func (c *Config) SetSlotNodeRef(slotID int, ref SlotNodeRef) {
	if c == nil {
		return
	}
	ref.SubID = strings.TrimSpace(ref.SubID)
	ref.NodeName = strings.TrimSpace(ref.NodeName)
	ref.NodeServer = strings.TrimSpace(ref.NodeServer)
	ref.NodeType = strings.TrimSpace(ref.NodeType)

	idx := c.slotIndex(slotID)
	if ref.SubID == "" && ref.NodeName == "" && ref.NodeServer == "" && ref.NodePort == 0 && ref.NodeType == "" {
		if idx >= 0 {
			c.Slots[idx].SubID = ""
			c.Slots[idx].NodeName = ""
			c.Slots[idx].NodeServer = ""
			c.Slots[idx].NodePort = 0
			c.Slots[idx].NodeType = ""
			c.normalizeSlots()
		}
		return
	}

	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.SubID = ref.SubID
	slot.NodeName = ref.NodeName
	slot.NodeServer = ref.NodeServer
	slot.NodePort = ref.NodePort
	slot.NodeType = ref.NodeType
	c.normalizeSlots()
}

// GetSlotStartMode returns the persisted startup mode for a slot.
// Defaults to "rule".
func (c *Config) GetSlotStartMode(slotID int) string {
	if c == nil {
		return "rule"
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		if mode := normalizeSlotStartMode(c.Slots[idx].StartMode); mode != "" {
			return mode
		}
	}
	return "rule"
}

// SetSlotStartMode sets the startup mode for a slot.
func (c *Config) SetSlotStartMode(slotID int, mode string) {
	if c == nil {
		return
	}
	mode = normalizeSlotStartMode(mode)
	if mode == "" {
		mode = "rule"
	}
	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.StartMode = mode
	c.normalizeSlots()
}

// GetSlotPortMode returns the port allocation mode for a slot.
// Defaults to "auto".
func (c *Config) GetSlotPortMode(slotID int) string {
	if c == nil {
		return "auto"
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		if mode := normalizeSlotPortMode(c.Slots[idx].PortMode); mode != "" {
			return mode
		}
	}
	return "auto"
}

// GetSlotManualMixedPort returns the manual mixed port for a slot, if configured.
func (c *Config) GetSlotManualMixedPort(slotID int) (int, bool) {
	if c == nil {
		return 0, false
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		port := c.Slots[idx].ManualMixedPort
		if port > 0 && port < 65536 {
			return port, true
		}
	}
	return 0, false
}

// SetSlotPortMode sets the port allocation mode for a slot.
func (c *Config) SetSlotPortMode(slotID int, mode string) {
	if c == nil {
		return
	}
	mode = normalizeSlotPortMode(mode)
	if mode == "" {
		mode = "auto"
	}
	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.PortMode = mode
	if mode != "manual" {
		slot.ManualMixedPort = 0
	}
	c.normalizeSlots()
}

// SetSlotManualMixedPort sets the manual mixed port and switches the slot to manual mode.
func (c *Config) SetSlotManualMixedPort(slotID int, port int) {
	if c == nil {
		return
	}
	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.PortMode = "manual"
	if port <= 0 || port >= 65536 {
		slot.ManualMixedPort = 0
		c.normalizeSlots()
		return
	}
	slot.ManualMixedPort = port
	c.normalizeSlots()
}

// SlotMixedPort returns the effective mixed port for a slot.
// The second return value reports whether the slot uses manual mode.
// The third return value reports whether the resolved configuration is valid.
func (c *Config) SlotMixedPort(slotID int) (int, bool, bool) {
	baseMixed := 17890
	if c != nil && c.Proxy.BaseMixedPort > 0 {
		baseMixed = c.Proxy.BaseMixedPort
	} else if c != nil && c.Proxy.MixedPort > 0 {
		baseMixed = c.Proxy.MixedPort
	}

	if c == nil || c.GetSlotPortMode(slotID) != "manual" {
		return baseMixed + slotID, false, true
	}

	port, ok := c.GetSlotManualMixedPort(slotID)
	if !ok {
		return 0, true, false
	}
	return port, true, true
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

// GetSlotLastLatency returns the cached latency for a slot (ms). 0 means no result.
func (c *Config) GetSlotLastLatency(slotID int) int64 {
	if c == nil {
		return 0
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		return c.Slots[idx].LastLatency
	}
	return 0
}

// SetSlotLastLatency sets the cached latency for a slot.
func (c *Config) SetSlotLastLatency(slotID int, latency int64) {
	if c == nil {
		return
	}
	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.LastLatency = latency
}

// GetSlotLastRegion returns the cached region for a slot.
func (c *Config) GetSlotLastRegion(slotID int) string {
	if c == nil {
		return ""
	}
	if idx := c.slotIndex(slotID); idx >= 0 {
		return c.Slots[idx].LastRegion
	}
	return ""
}

// SetSlotLastRegion sets the cached region for a slot.
func (c *Config) SetSlotLastRegion(slotID int, region string) {
	if c == nil {
		return
	}
	slot := c.ensureSlotConfig(slotID)
	if slot == nil {
		return
	}
	slot.LastRegion = region
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
