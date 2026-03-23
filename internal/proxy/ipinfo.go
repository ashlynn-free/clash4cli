package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// IPInfoResult holds parsed ipinfo.io response fields.
type IPInfoResult struct {
	IP       string `json:"ip"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Org      string `json:"org"`
	Timezone string `json:"timezone"`
	Error    string `json:"-"`
}

var fetchIPInfoFunc = defaultFetchIPInfo

// SetFetchIPInfoForTests overrides the IP info fetcher for unit tests.
func SetFetchIPInfoForTests(fn func(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult) (restore func()) {
	prev := fetchIPInfoFunc
	if fn == nil {
		fetchIPInfoFunc = defaultFetchIPInfo
	} else {
		fetchIPInfoFunc = fn
	}
	return func() { fetchIPInfoFunc = prev }
}

// ProbeIPInfoViaProxy fetches IP info from ipinfo.io through the given HTTP proxy.
func ProbeIPInfoViaProxy(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult {
	return fetchIPInfoFunc(proxyHost, proxyPort, timeout)
}

func defaultFetchIPInfo(proxyHost string, proxyPort int, timeout time.Duration) IPInfoResult {
	proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%d", proxyHost, proxyPort))
	if err != nil {
		return IPInfoResult{Error: err.Error()}
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true},
	}

	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return IPInfoResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return IPInfoResult{Error: err.Error()}
	}

	if resp.StatusCode != http.StatusOK {
		return IPInfoResult{Error: fmt.Sprintf("ipinfo returned %d", resp.StatusCode)}
	}

	var result IPInfoResult
	if err := json.Unmarshal(body, &result); err != nil {
		return IPInfoResult{Error: err.Error()}
	}
	return result
}
