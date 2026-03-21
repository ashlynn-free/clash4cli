package i18n

import (
	"embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed lang/*.yaml
var langFS embed.FS

// Messages holds all translated strings
type Messages struct {
	App          AppMessages          `yaml:"app"`
	Tabs         TabMessages          `yaml:"tabs"`
	Nodes        NodeMessages         `yaml:"nodes"`
	NodeDetail   NodeDetailMessages   `yaml:"node_detail"`
	Subscription SubscriptionMessages `yaml:"subscription"`
	Settings     SettingsMessages     `yaml:"settings"`
	Slots        SlotsMessages        `yaml:"slots"`
	Logs         LogMessages          `yaml:"logs"`
	Status       StatusMessages       `yaml:"status"`
	Actions      ActionMessages       `yaml:"actions"`
	Errors       ErrorMessages        `yaml:"errors"`
}

type AppMessages struct {
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
	Quit    string `yaml:"quit"`
	Help    string `yaml:"help"`
}

type TabMessages struct {
	Nodes        string `yaml:"nodes"`
	Slots        string `yaml:"slots"`
	Subscription string `yaml:"subscription"`
	Settings     string `yaml:"settings"`
	Logs         string `yaml:"logs"`
}

type NodeMessages struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Latency   string `yaml:"latency"`
	Speed     string `yaml:"speed"`
	Status    string `yaml:"status"`
	NoNodes   string `yaml:"no_nodes"`
	Testing   string `yaml:"testing"`
	Select    string `yaml:"select"`
	TestAll   string `yaml:"test_all"`
	Connected string `yaml:"connected"`
	Timeout   string `yaml:"timeout"`
	Unknown   string `yaml:"unknown"`
}

type NodeDetailMessages struct {
	Title        string `yaml:"title"`
	Server       string `yaml:"server"`
	Settings     string `yaml:"settings"`
	WebsiteTests string `yaml:"website_tests"`
	Testing      string `yaml:"testing"`
	NotTested    string `yaml:"not_tested"`
	NoProxy      string `yaml:"no_proxy"`
	IPInfo       string `yaml:"ip_info"`
	Location     string `yaml:"location"`
	Org          string `yaml:"org"`
}

type SubscriptionMessages struct {
	Name         string `yaml:"name"`
	URL          string `yaml:"url"`
	NodeCount    string `yaml:"node_count"`
	LastUpdate   string `yaml:"last_update"`
	Add          string `yaml:"add"`
	Delete       string `yaml:"delete"`
	Update       string `yaml:"update"`
	UpdateAll    string `yaml:"update_all"`
	NoSubs       string `yaml:"no_subs"`
	Updating     string `yaml:"updating"`
	InputURL     string `yaml:"input_url"`
	InputName    string `yaml:"input_name"`
	InputContent string `yaml:"input_content"`
}

type SettingsMessages struct {
	MixedPortLabel string `yaml:"mixed_port_label"`
	MixedPortHelp  string `yaml:"mixed_port_help"`
	SocksPort      string `yaml:"socks_port"`
	HTTPPort       string `yaml:"http_port"`
	AllowLan       string `yaml:"allow_lan"`
	LinuxFeatures  string `yaml:"linux_features"`
	Tun            string `yaml:"tun"`
	DockerTCP      string `yaml:"docker_tcp"`
	Language       string `yaml:"language"`
	Concurrent     string `yaml:"concurrent"`
	Timeout        string `yaml:"timeout"`
	ServerDisplay  string `yaml:"server_display"`
	RestartCore    string `yaml:"restart_core"`
	On             string `yaml:"on"`
	Off            string `yaml:"off"`
	Save           string `yaml:"save"`
	Saved          string `yaml:"saved"`
}

type SlotsMessages struct {
	Title              string `yaml:"title"`
	NoProxy            string `yaml:"no_proxy"`
	NoUpstream         string `yaml:"no_upstream"`
	Power              string `yaml:"power"`
	Mode               string `yaml:"mode"`
	Proxy              string `yaml:"proxy"`
	Port               string `yaml:"port"`
	Upstream           string `yaml:"upstream"`
	AutoPort           string `yaml:"auto_port"`
	ManualPort         string `yaml:"manual_port"`
	ManualPortPrompt   string `yaml:"manual_port_prompt"`
	SelectProxy        string `yaml:"select_proxy"`
	SelectMode         string `yaml:"select_mode"`
	SelectPort         string `yaml:"select_port"`
	SelectUpstream     string `yaml:"select_upstream"`
	Ready              string `yaml:"ready"`
	Running            string `yaml:"running"`
	MissingProxy       string `yaml:"missing_proxy"`
	InvalidPort        string `yaml:"invalid_port"`
	Started            string `yaml:"started"`
	Stopped            string `yaml:"stopped"`
	Restarted          string `yaml:"restarted"`
	Saved              string `yaml:"saved"`
	Restarting         string `yaml:"restarting"`
	ProxyPickerHint    string `yaml:"proxy_picker_hint"`
	UpstreamPickerHint string `yaml:"upstream_picker_hint"`
}

type LogMessages struct {
	Clear   string `yaml:"clear"`
	NoLogs  string `yaml:"no_logs"`
	Level   string `yaml:"level"`
	Time    string `yaml:"time"`
	Message string `yaml:"message"`
}

type StatusMessages struct {
	Disconnected    string `yaml:"disconnected"`
	Connecting      string `yaml:"connecting"`
	Connected       string `yaml:"connected"`
	ConnectedRule   string `yaml:"connected_rule"`
	ConnectedGlobal string `yaml:"connected_global"`
	Error           string `yaml:"error"`
	ProxyInfo       string `yaml:"proxy_info"`
}

type ActionMessages struct {
	Confirm  string `yaml:"confirm"`
	Cancel   string `yaml:"cancel"`
	Back     string `yaml:"back"`
	Enter    string `yaml:"enter"`
	Tab      string `yaml:"tab"`
	Arrows   string `yaml:"arrows"`
	LrSwitch string `yaml:"lr_switch"`
}

type ErrorMessages struct {
	LoadConfig     string `yaml:"load_config"`
	SaveConfig     string `yaml:"save_config"`
	FetchSub       string `yaml:"fetch_sub"`
	ParseSub       string `yaml:"parse_sub"`
	StartProxy     string `yaml:"start_proxy"`
	StopProxy      string `yaml:"stop_proxy"`
	InvalidPort    string `yaml:"invalid_port"`
	NoNodeSelected string `yaml:"no_node_selected"`
}

var currentLang = "en_us"
var messages *Messages
var fallbackMessages *Messages

// Init initializes the i18n system with the specified language
func Init(lang string) error {
	// Always load fallback first
	var err error
	fallbackMessages, err = loadLang("en_us")
	if err != nil {
		return fmt.Errorf("failed to load fallback language: %w", err)
	}

	// Try to load requested language
	messages, err = loadLang(lang)
	if err != nil {
		// Fall back to English
		messages = fallbackMessages
		currentLang = "en_us"
	} else {
		currentLang = lang
	}

	return nil
}

func loadLang(lang string) (*Messages, error) {
	filename := fmt.Sprintf("lang/%s.yaml", strings.ToLower(lang))
	data, err := langFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var msgs Messages
	if err := yaml.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}

	return &msgs, nil
}

// GetMessages returns the current messages
func GetMessages() *Messages {
	if messages == nil {
		_ = Init("en_us")
	}
	return messages
}

// CurrentLanguage returns the current language code
func CurrentLanguage() string {
	return currentLang
}

// SetLanguage changes the current language
func SetLanguage(lang string) error {
	return Init(lang)
}

// M is a shortcut to get messages
func M() *Messages {
	return GetMessages()
}
