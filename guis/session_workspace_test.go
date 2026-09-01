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

func TestSessionWorkspaceCreatesIndependentRuntimeSessionsAndPreservesPerTabState(t *testing.T) {
	workspace := newSessionWorkspace()
	local := connectionProfile{ID: "local", Name: "本地", Type: connectionTypeLocal}
	server := connectionProfile{ID: "server", Name: "Сервер", Type: connectionTypeSSH}

	firstLocalIndex, created := workspace.Open(local)
	if !created || firstLocalIndex != 0 {
		t.Fatalf("first local session was not created: index=%d created=%t", firstLocalIndex, created)
	}
	firstLocal, _ := workspace.Active()
	workspace.SetActiveDraft("printf '简体 · 繁體 · Русский'")
	workspace.AppendActiveOutput("本地输出\n")
	workspace.Open(server)
	workspace.SetActiveDraft("uname -a")
	workspace.AppendActiveOutput("сервер\n")

	secondLocalIndex, created := workspace.Open(local)
	if !created || secondLocalIndex != 2 {
		t.Fatalf("same profile must create an independent runtime session: index=%d created=%t", secondLocalIndex, created)
	}
	secondLocal, _ := workspace.Active()
	if firstLocal.ID == secondLocal.ID || firstLocal.ID == local.ID || secondLocal.ID == local.ID {
		t.Fatalf("runtime IDs must be unique and independent from profile IDs: first=%q second=%q", firstLocal.ID, secondLocal.ID)
	}
	if firstLocal.ProfileID != local.ID || secondLocal.ProfileID != local.ID {
		t.Fatalf("runtime sessions lost source profile identity: first=%#v second=%#v", firstLocal, secondLocal)
	}
	if firstLocal.InstanceNumber != 1 || secondLocal.InstanceNumber != 2 {
		t.Fatalf("unexpected runtime instance numbers: first=%d second=%d", firstLocal.InstanceNumber, secondLocal.InstanceNumber)
	}
	if secondLocal.CommandDraft != "" || strings.Contains(secondLocal.Output, "本地输出\n") {
		t.Fatalf("new runtime session inherited mutable state: %#v", secondLocal)
	}

	workspace.Select(firstLocalIndex)
	active, _ := workspace.Active()
	if active.CommandDraft != "printf '简体 · 繁體 · Русский'" || !strings.Contains(active.Output, "本地输出\n") {
		t.Fatalf("first local runtime state was not preserved: %#v", active)
	}
	if len(workspace.Tabs()) != 3 {
		t.Fatalf("expected three independent runtime sessions: %#v", workspace.Tabs())
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

func TestSessionWorkspaceRuntimeIdentitySurvivesEarlierTabClose(t *testing.T) {
	workspace := newSessionWorkspace()
	profile := connectionProfile{ID: "same-profile", Name: "Server", Type: connectionTypeSSH}
	workspace.Open(profile)
	workspace.Open(profile)
	if !workspace.Close(0) {
		t.Fatal("failed to close first runtime session")
	}
	workspace.Open(profile)
	tabs := workspace.Tabs()
	if len(tabs) != 2 || tabs[0].InstanceNumber != 2 || tabs[1].InstanceNumber != 3 {
		t.Fatalf("runtime instance numbers were reused after close: %#v", tabs)
	}
	if tabs[0].ID == tabs[1].ID {
		t.Fatalf("runtime ID was reused after close: %#v", tabs)
	}
	if got := sessionTabTitle(tabs[1]); got != "Server · 3" {
		t.Fatalf("duplicate profile tab title = %q, want %q", got, "Server · 3")
	}
}

func TestSessionWorkspaceReopensMostRecentlyClosedProfileAsFreshRuntime(t *testing.T) {
	workspace := newSessionWorkspace()
	first := connectionProfile{ID: "first", Name: "First", Type: connectionTypeLocal}
	second := connectionProfile{ID: "second", Name: "Second", Type: connectionTypeSSH}
	workspace.Open(first)
	firstRuntime, _ := workspace.Active()
	workspace.Open(second)
	secondRuntime, _ := workspace.Active()

	if !workspace.Close(workspace.ActiveIndex()) || !workspace.Close(workspace.ActiveIndex()) {
		t.Fatal("failed to close runtime sessions")
	}
	if _, ok := workspace.Active(); ok {
		t.Fatal("workspace should be empty before reopen")
	}

	index, reopened := workspace.ReopenLastClosed()
	if !reopened || index != 0 {
		t.Fatalf("reopen latest profile: index=%d reopened=%t", index, reopened)
	}
	active, _ := workspace.Active()
	if active.ProfileID != first.ID || active.ID == firstRuntime.ID || active.CommandDraft != "" {
		t.Fatalf("reopened session is not a fresh runtime for the latest profile: %#v", active)
	}
	if !workspace.Close(index) {
		t.Fatal("failed to close reopened profile")
	}
	if _, reopened = workspace.ReopenLastClosed(); !reopened {
		t.Fatal("failed to reopen the profile a second time")
	}
	active, _ = workspace.Active()
	if active.ProfileID != first.ID || active.ID == secondRuntime.ID {
		t.Fatalf("closing a reopened tab did not update the LIFO stack: %#v", active)
	}
}

func TestSessionWorkspaceReopenRejectsEmptyHistory(t *testing.T) {
	workspace := newSessionWorkspace()
	if index, ok := workspace.ReopenLastClosed(); ok || index != -1 {
		t.Fatalf("empty reopen = index %d, ok %t", index, ok)
	}
}
