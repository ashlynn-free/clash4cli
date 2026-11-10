package tabs

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"rocket4cli/internal/node"
	"rocket4cli/internal/proxy"
	"rocket4cli/internal/subscription"
)

func TestSubscriptionsTab_EnterOpensNodeBrowser(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	subMgr := subscription.NewManager()
	_, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#NodeA\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	nodeMgr := node.NewManager(subMgr, 1, 0)
	tab := NewSubscriptionsTab(subMgr, &proxy.SlotManager{}, nodeMgr)
	tab.SetSize(120, 30)

	// Load subscriptions rows.
	tab, _ = tab.Update(SubsRefreshedMsg{Subs: subMgr.List()})
	if len(tab.subs) == 0 {
		t.Fatalf("expected subscriptions loaded")
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !tab.showNodeBrowser {
		t.Fatalf("expected node browser to open")
	}
	if !tab.BlocksGlobalKeys() {
		t.Fatalf("expected global keys blocked when node browser is open")
	}
	if view := tab.View(); view == "" {
		t.Fatalf("expected non-empty node browser view")
	}
}

func TestSubscriptionsTab_EscClosesNodeBrowser(t *testing.T) {
	t.Setenv("CLASH4CLI_HOME", t.TempDir())
	subMgr := subscription.NewManager()
	_, err := subMgr.AddManual("sub", "ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#NodeA\n")
	if err != nil {
		t.Fatalf("AddManual: %v", err)
	}

	nodeMgr := node.NewManager(subMgr, 1, 0)
	tab := NewSubscriptionsTab(subMgr, &proxy.SlotManager{}, nodeMgr)
	tab.SetSize(120, 30)
	tab, _ = tab.Update(SubsRefreshedMsg{Subs: subMgr.List()})
	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !tab.showNodeBrowser {
		t.Fatalf("expected node browser to open")
	}

	tab, _ = tab.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if tab.showNodeBrowser {
		t.Fatalf("expected node browser to close on esc")
	}
}
