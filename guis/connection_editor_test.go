package guis

import "testing"

func TestConnectionEditorDraftBuildsProfileWithoutTouchingRecencyOrStoredSecret(t *testing.T) {
	base := connectionProfile{
		ID:          "prod",
		Name:        "Old",
		Group:       "Legacy",
		Type:        connectionTypeSSH,
		Host:        "old.example.com",
		Port:        22,
		Username:    "old-user",
		PasswordEnc: "stored-secret",
		LastUsed:    "2026-08-03T10:00:00Z",
	}
	draft := connectionEditorDraft{
		Name:       "  生产 API  ",
		Group:      "  生产环境  ",
		Type:       "SSH",
		Host:       " api.example.com ",
		Port:       "2222",
		Username:   " deploy ",
		WorkingDir: " /srv/api ",
		PrivateKey: " ~/.ssh/id_ed25519 ",
	}

	got, err := draft.Profile(base, "")
	if err != nil {
		t.Fatalf("Profile failed: %v", err)
	}
	if got.ID != base.ID || got.Name != "生产 API" || got.Group != "生产环境" || got.Type != connectionTypeSSH ||
		got.Host != "api.example.com" || got.Port != 2222 || got.Username != "deploy" || got.WorkingDir != "/srv/api" ||
		got.PrivateKey != "~/.ssh/id_ed25519" {
		t.Fatalf("unexpected profile: %#v", got)
	}
	if got.PasswordEnc != base.PasswordEnc {
		t.Fatalf("empty password draft replaced stored secret: %#v", got)
	}
	if got.LastUsed != base.LastUsed {
		t.Fatalf("editing fabricated recency: got %q want %q", got.LastUsed, base.LastUsed)
	}
}

func TestConnectionEditorDraftValidatesTypeAndPort(t *testing.T) {
	base := connectionProfile{ID: "target", Type: connectionTypeSSH}
	for _, tc := range []struct {
		name  string
		draft connectionEditorDraft
	}{
		{name: "bad type", draft: connectionEditorDraft{Name: "Target", Group: "Default", Type: "telnet", Port: "22"}},
		{name: "bad port", draft: connectionEditorDraft{Name: "Target", Group: "Default", Type: "ssh", Port: "abc"}},
		{name: "port too high", draft: connectionEditorDraft{Name: "Target", Group: "Default", Type: "ssh", Port: "65536"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.draft.Profile(base, ""); err == nil {
				t.Fatalf("invalid draft accepted: %#v", tc.draft)
			}
		})
	}
}

func TestConnectionEditorDraftUsesLocalizedDefaultsForBlankFields(t *testing.T) {
	got, err := (connectionEditorDraft{Type: "local"}).Profile(connectionProfile{ID: "new"}, "")
	if err != nil {
		t.Fatalf("Profile failed: %v", err)
	}
	if got.Name != tr("profile.unnamed") || got.Group != tr("profile.default_group") || got.Port != 0 {
		t.Fatalf("defaults mismatch: %#v", got)
	}
}

func TestConnectionEditorFieldPolicySeparatesLocalAndSSHFields(t *testing.T) {
	local := connectionEditorFieldPolicy(connectionTypeLocal)
	if local.Host || local.Port || local.Username || local.Password || local.PrivateKey || !local.WorkingDir {
		t.Fatalf("local editor field policy = %#v", local)
	}

	ssh := connectionEditorFieldPolicy(connectionTypeSSH)
	if !ssh.Host || !ssh.Port || !ssh.Username || !ssh.Password || !ssh.PrivateKey || !ssh.WorkingDir {
		t.Fatalf("SSH editor field policy = %#v", ssh)
	}
}

func TestConnectionEditorTypeOptionsStayCanonicalAcrossLocalizedLabels(t *testing.T) {
	options := connectionEditorTypeOptions()
	if len(options) != 2 || options[0].Type != connectionTypeLocal || options[1].Type != connectionTypeSSH {
		t.Fatalf("connection type options = %#v", options)
	}
	if options[0].Label == "" || options[1].Label == "" {
		t.Fatalf("connection type labels must be visible: %#v", options)
	}
}

func TestConnectionEditorReplacementSeparatesFocusFromTargetSwitch(t *testing.T) {
	current := &connectionEditor{profile: connectionProfile{ID: "alpha"}}
	if current.queueReplacement(connectionProfile{ID: "alpha", Name: "refreshed copy"}) {
		t.Fatal("opening the current profile should focus the existing editor, not replace its draft")
	}
	if current.pendingReplacement != nil {
		t.Fatalf("same-target focus queued a replacement: %#v", current.pendingReplacement)
	}

	next := connectionProfile{ID: "beta", Name: "Beta"}
	if !current.queueReplacement(next) {
		t.Fatal("opening another profile should request an explicit editor target switch")
	}
	got, ok := current.takePendingReplacement()
	if !ok || got.ID != next.ID || got.Name != next.Name {
		t.Fatalf("pending replacement = (%#v, %v), want %#v", got, ok, next)
	}
	if _, ok := current.takePendingReplacement(); ok {
		t.Fatal("replacement must be consumed exactly once")
	}
}

func TestConnectionEditorReplacementCanBeCancelledWhenDraftIsKept(t *testing.T) {
	editor := &connectionEditor{profile: connectionProfile{ID: "alpha"}}
	if !editor.queueReplacement(connectionProfile{ID: "beta"}) {
		t.Fatal("different target was not queued")
	}
	editor.cancelPendingReplacement()
	if _, ok := editor.takePendingReplacement(); ok {
		t.Fatal("Keep Editing must cancel the queued target switch")
	}
}

func TestConnectionEditorCannotResurrectDeletedExistingProfile(t *testing.T) {
	rows := []connectionProfile{{ID: "remaining"}}
	if canPersistEditorProfile(rows, "deleted", false) {
		t.Fatal("an editor for a deleted saved profile must not recreate it")
	}
	if !canPersistEditorProfile(rows, "remaining", false) {
		t.Fatal("an editor for an existing saved profile should remain saveable")
	}
	if !canPersistEditorProfile(rows, "new-profile", true) {
		t.Fatal("a new-profile editor should be allowed to create its profile")
	}
}

func TestConnectionEditorSerializesTargetDeletionAfterAcceptedClose(t *testing.T) {
	editor := &connectionEditor{profile: connectionProfile{ID: "alpha"}}
	if !editor.queueReplacement(connectionProfile{ID: "beta"}) {
		t.Fatal("test setup did not queue replacement")
	}
	called := false
	if !editor.queueAfterCloseForProfile("alpha", func() { called = true }) {
		t.Fatal("matching deletion target was not queued")
	}
	if editor.pendingReplacement != nil {
		t.Fatal("destructive target action must supersede a stale editor replacement")
	}
	action := editor.takePendingAfterClose()
	if action == nil {
		t.Fatal("accepted close lost its pending deletion action")
	}
	action()
	if !called || editor.takePendingAfterClose() != nil {
		t.Fatal("pending deletion action must run and be consumed exactly once")
	}
	if editor.queueAfterCloseForProfile("beta", func() {}) {
		t.Fatal("another profile must not close this editor")
	}

	editor.queueAfterCloseForProfile("alpha", func() {})
	editor.cancelPendingCloseWork()
	if editor.takePendingAfterClose() != nil {
		t.Fatal("Keep Editing must cancel pending deletion")
	}
}
