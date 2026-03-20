package subscription

import (
	"time"
)

// Subscription represents a proxy subscription
type Subscription struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	// Source indicates how this subscription is provided:
	// - "url": fetched from URL
	// - "manual": user pasted raw content
	// - "custom": local, user-managed nodes under the reserved "Cus" subscription
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	URL    string `yaml:"url,omitempty" json:"url,omitempty"`
	// Content stores user-pasted raw subscription content (base64 / Clash YAML / SIP008 JSON).
	// It is only used when Source == "manual".
	Content             string    `yaml:"content,omitempty" json:"content,omitempty"`
	Nodes               []Node    `yaml:"nodes" json:"nodes"`
	LastUpdateAttemptAt time.Time `yaml:"last_update_attempt_at,omitempty" json:"last_update_attempt_at,omitempty"`
	LastUpdateError     string    `yaml:"last_update_error,omitempty" json:"last_update_error,omitempty"`
	LastUpdateViaProxy  bool      `yaml:"last_update_via_proxy,omitempty" json:"last_update_via_proxy,omitempty"`
	UpdatedAt           time.Time `yaml:"updated_at" json:"updated_at"`
	CreatedAt           time.Time `yaml:"created_at" json:"created_at"`
}

// Node represents a proxy node
type Node struct {
	Name     string         `yaml:"name" json:"name"`
	Type     string         `yaml:"type" json:"type"`
	Server   string         `yaml:"server" json:"server"`
	Port     int            `yaml:"port" json:"port"`
	Settings map[string]any `yaml:"settings,omitempty" json:"settings,omitempty"`

	// Runtime fields (not persisted)
	Latency int64   `yaml:"-" json:"-"` // in milliseconds, -1 means timeout
	Speed   int64   `yaml:"-" json:"-"` // in bytes per second
	Loss    float64 `yaml:"-" json:"-"` // packet loss percentage
}

// SubFormat represents the subscription format type
type SubFormat int

const (
	CustomSubscriptionID   = "__custom__"
	CustomSubscriptionName = "Cus"
	SourceURL              = "url"
	SourceManual           = "manual"
	SourceCustom           = "custom"
)

func IsCustomSubscriptionID(id string) bool {
	return id == CustomSubscriptionID
}

func IsCustomSubscription(sub *Subscription) bool {
	return sub != nil && IsCustomSubscriptionID(sub.ID)
}

const (
	FormatUnknown SubFormat = iota
	FormatClashYAML
	FormatBase64
	FormatSIP008
	FormatURIList
)

// String returns the string representation of SubFormat
func (f SubFormat) String() string {
	switch f {
	case FormatClashYAML:
		return "Clash YAML"
	case FormatBase64:
		return "Base64"
	case FormatSIP008:
		return "SIP008"
	case FormatURIList:
		return "URI List"
	default:
		return "Unknown"
	}
}

// NodeType constants
const (
	TypeSS        = "ss"
	TypeSSR       = "ssr"
	TypeVMess     = "vmess"
	TypeVLESS     = "vless"
	TypeTrojan    = "trojan"
	TypeHysteria  = "hysteria"
	TypeHysteria2 = "hysteria2"
	TypeTUIC      = "tuic"
	TypeWireGuard = "wireguard"
	TypeHTTP      = "http"
	TypeSOCKS5    = "socks5"
)

// IsValidType checks if the node type is supported
func IsValidType(t string) bool {
	validTypes := []string{
		TypeSS, TypeSSR, TypeVMess, TypeVLESS, TypeTrojan,
		TypeHysteria, TypeHysteria2, TypeTUIC, TypeWireGuard,
		TypeHTTP, TypeSOCKS5,
	}
	for _, vt := range validTypes {
		if t == vt {
			return true
		}
	}
	return false
}
