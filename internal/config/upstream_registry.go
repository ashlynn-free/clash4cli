package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// GetUpstreamProxy returns the upstream proxy entry with the given ID.
func (c *Config) GetUpstreamProxy(id string) (UpstreamProxyEntry, bool) {
	if c == nil {
		return UpstreamProxyEntry{}, false
	}
	for _, entry := range c.UpstreamProxies {
		if entry.ID == id {
			return entry, true
		}
	}
	return UpstreamProxyEntry{}, false
}

// AddUpstreamProxy adds a new upstream proxy entry. If entry.ID is empty a random
// 8-character hex ID is generated. Returns the assigned ID.
func (c *Config) AddUpstreamProxy(entry UpstreamProxyEntry) string {
	if entry.ID == "" {
		entry.ID = generateID()
	}
	entry = entry.normalized()
	c.UpstreamProxies = append(c.UpstreamProxies, entry)
	return entry.ID
}

// UpdateUpstreamProxy updates an existing upstream proxy entry by ID.
func (c *Config) UpdateUpstreamProxy(entry UpstreamProxyEntry) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	entry = entry.normalized()
	for i := range c.UpstreamProxies {
		if c.UpstreamProxies[i].ID == entry.ID {
			c.UpstreamProxies[i] = entry
			return nil
		}
	}
	return fmt.Errorf("upstream proxy %q not found", entry.ID)
}

// DeleteUpstreamProxy removes an upstream proxy entry. Returns an error if any
// slots still reference it.
func (c *Config) DeleteUpstreamProxy(id string) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	users := c.UpstreamProxyUsedBySlots(id)
	if len(users) > 0 {
		return fmt.Errorf("upstream proxy %q is still used by slot(s) %v", id, users)
	}
	for i := range c.UpstreamProxies {
		if c.UpstreamProxies[i].ID == id {
			c.UpstreamProxies = append(c.UpstreamProxies[:i], c.UpstreamProxies[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("upstream proxy %q not found", id)
}

// ListUpstreamProxies returns all upstream proxy entries.
func (c *Config) ListUpstreamProxies() []UpstreamProxyEntry {
	if c == nil {
		return nil
	}
	return c.UpstreamProxies
}

// UpstreamProxyUsedBySlots returns the list of slot IDs referencing a given upstream ID.
func (c *Config) UpstreamProxyUsedBySlots(id string) []int {
	if c == nil {
		return nil
	}
	var slots []int
	for _, slot := range c.Slots {
		if slot.UpstreamID == id {
			slots = append(slots, slot.ID)
		}
	}
	return slots
}

func generateID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// fallback: should never happen in practice
		return "00000000"
	}
	return hex.EncodeToString(b)
}

func (e UpstreamProxyEntry) normalized() UpstreamProxyEntry {
	e.Label = strings.TrimSpace(e.Label)
	e.Protocol = strings.ToLower(strings.TrimSpace(e.Protocol))
	e.Server = strings.TrimSpace(e.Server)
	e.Username = strings.TrimSpace(e.Username)
	e.Password = strings.TrimSpace(e.Password)
	return e
}
