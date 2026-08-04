package guis

import (
	"strings"
	"testing"
)

func TestSessionWorkspaceKeepsProfileSelectionSeparateFromActiveTab(t *testing.T) {
	workspace := newSessionWorkspace()
	local := connectionProfile{ID: "local", Name: "Local", Type: connectionTypeLocal}
	server := connectionProfile{ID: "server", Name: "Server", Type: connectionTypeSSH}

	localIndex, created := workspace.Open(local)
	if !created || localIndex != 0 {
		t.Fatalf("unexpected local open: index=%d created=%t", localIndex, created)
	}
	serverIndex, created := workspace.Open(server)
	if !created || serverIndex != 1 {
		t.Fatalf("unexpected server open: index=%d created=%t", serverIndex, created)
	}
	workspace.Select(localIndex)
	workspace.SetProfileSelection(server.ID)

	active, ok := workspace.Active()
	if !ok || active.Profile.ID != local.ID {
		t.Fatalf("profile selection changed active terminal tab: %#v", active)
	}
	if workspace.ProfileSelection() != server.ID {
		t.Fatalf("profile selection not retained separately: %q", workspace.ProfileSelection())
	}
}

func TestSessionWorkspaceReusesConnectionTabAndPreservesPerTabDraftOutput(t *testing.T) {
	workspace := newSessionWorkspace()
	local := connectionProfile{ID: "local", Name: "本地", Type: connectionTypeLocal}
	server := connectionProfile{ID: "server", Name: "Сервер", Type: connectionTypeSSH}

	workspace.Open(local)
	workspace.SetActiveDraft("printf '简体 · 繁體 · Русский'")
	workspace.AppendActiveOutput("本地输出\n")
	workspace.Open(server)
	workspace.SetActiveDraft("uname -a")
	workspace.AppendActiveOutput("сервер\n")

	index, created := workspace.Open(local)
	if created || index != 0 {
		t.Fatalf("existing profile should select its tab: index=%d created=%t", index, created)
	}
	active, _ := workspace.Active()
	if active.CommandDraft != "printf '简体 · 繁體 · Русский'" || !strings.Contains(active.Output, "本地输出\n") {
		t.Fatalf("local tab state was not preserved: %#v", active)
	}
	if len(workspace.Tabs()) != 2 {
		t.Fatalf("duplicate tab was created: %#v", workspace.Tabs())
	}
}

func TestSessionWorkspaceRunningLifecycleAndClosePolicy(t *testing.T) {
	workspace := newSessionWorkspace()
	workspace.Open(connectionProfile{ID: "one", Name: "One", Type: connectionTypeLocal})
	workspace.Open(connectionProfile{ID: "two", Name: "Two", Type: connectionTypeLocal})
	workspace.Select(0)

	if !workspace.BeginRun("run-1") {
		t.Fatal("BeginRun failed")
	}
	if workspace.Close(0) {
		t.Fatal("running session tab must not close")
	}
	if !workspace.FinishRun("run-1", sessionSucceeded) {
		t.Fatal("FinishRun failed")
	}
	if !workspace.Close(0) {
		t.Fatal("idle completed session should close")
	}
	active, ok := workspace.Active()
	if !ok || active.Profile.ID != "two" || workspace.ActiveIndex() != 0 {
		t.Fatalf("nearest session was not activated after close: active=%#v index=%d", active, workspace.ActiveIndex())
	}
}

func TestSessionWorkspaceRejectsStaleCompletion(t *testing.T) {
	workspace := newSessionWorkspace()
	workspace.Open(connectionProfile{ID: "local", Name: "Local", Type: connectionTypeLocal})
	if !workspace.BeginRun("new-run") {
		t.Fatal("BeginRun failed")
	}
	if workspace.FinishRun("stale-run", sessionFailed) {
		t.Fatal("stale completion mutated active session")
	}
	active, _ := workspace.Active()
	if active.Status != sessionRunning || active.RunID != "new-run" {
		t.Fatalf("running state was corrupted: %#v", active)
	}
}
