package daemon

import (
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

type RuntimeSnapshot struct {
	APIVersion string             `json:"api_version"`
	Revision   uint64             `json:"revision"`
	Daemon     *AuthorityInfo     `json:"daemon"`
	Active     []*proxy.SlotState `json:"active"`
	Count      int                `json:"count"`
}

type SlotResponse = proxy.SlotState

type StartSlotRequest struct {
	SubID        string              `json:"sub_id"`
	Nodes        []subscription.Node `json:"nodes"`
	SelectedNode *subscription.Node  `json:"selected_node"`
	Mode         string              `json:"mode"`
}

type SetModeRequest struct {
	Mode string `json:"mode"`
}

