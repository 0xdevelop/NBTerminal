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
