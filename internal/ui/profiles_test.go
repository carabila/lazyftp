package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/MawCeron/lazyftp/internal/client"
	"github.com/MawCeron/lazyftp/internal/config"
)

func TestProfileFromPasswordFormStoresPassword(t *testing.T) {
	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthPassword
	bar.inputs[fieldHost].SetValue("sftp.example.org")
	bar.inputs[fieldUser].SetValue("alice")
	bar.inputs[fieldPass].SetValue("server secret")
	bar.inputs[fieldKeyPassphrase].SetValue("not a server password")

	profile := bar.profileFromForm("work")
	if profile.Password != "server secret" {
		t.Errorf("saved password = %q, want the SFTP password", profile.Password)
	}
	if profile.Protocol != "SFTP" || profile.Auth != config.AuthPassword {
		t.Errorf("saved profile protocol/auth = %q/%q", profile.Protocol, profile.Auth)
	}
}

func TestProfileFromIdentityFormStoresPathButNotPassphrases(t *testing.T) {
	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthIdentityFile
	bar.inputs[fieldHost].SetValue("sftp.example.org")
	bar.inputs[fieldUser].SetValue("alice")
	bar.inputs[fieldIdentity].SetValue("~/.ssh/work")
	bar.inputs[fieldPass].SetValue("stale server password")
	bar.inputs[fieldKeyPassphrase].SetValue("private key passphrase")

	profile := bar.profileFromForm("work")
	if profile.Auth != config.AuthIdentityFile || profile.IdentityFile != "~/.ssh/work" {
		t.Errorf("saved auth/path = %q/%q", profile.Auth, profile.IdentityFile)
	}
	if profile.Password != "" {
		t.Errorf("identity profile saved an unrelated password: %q", profile.Password)
	}
}

func TestSelectingIdentityProfileRestoresSettingsAndClearsKeyPassphrase(t *testing.T) {
	bar := NewConnectionBar()
	bar.inputs[fieldKeyPassphrase].SetValue("old passphrase")
	profile := config.Profile{
		Name:         "EMI",
		Protocol:     "SFTP",
		Host:         "sftp.example.org",
		User:         "alice",
		Auth:         config.AuthIdentityFile,
		IdentityFile: "~/.ssh/emi",
	}

	bar = bar.withProfile(profile)
	if bar.protocol != client.SFTP || bar.auth != client.SFTPAuthIdentityFile {
		t.Errorf("selected protocol/auth = %s/%d", bar.protocol, bar.auth)
	}
	if bar.inputs[fieldHost].Value() != profile.Host || bar.inputs[fieldUser].Value() != profile.User {
		t.Errorf("selected host/user = %q/%q", bar.inputs[fieldHost].Value(), bar.inputs[fieldUser].Value())
	}
	if bar.inputs[fieldIdentity].Value() != profile.IdentityFile {
		t.Errorf("selected identity path = %q, want %q", bar.inputs[fieldIdentity].Value(), profile.IdentityFile)
	}
	if bar.inputs[fieldKeyPassphrase].Value() != "" {
		t.Error("profile selection restored a key passphrase")
	}
	if bar.focused != fieldKeyPassphrase {
		t.Errorf("focus after loading identity profile = %d, want key passphrase", bar.focused)
	}
}

func TestSaveProfileShortcutBuildsPasswordProfileRequest(t *testing.T) {
	bar := NewConnectionBar().withProfilesLoaded(profilesLoadedMsg{})
	bar.protocol = client.SFTP
	bar.inputs[fieldHost].SetValue("sftp.example.org")
	bar.inputs[fieldUser].SetValue("alice")
	bar.inputs[fieldPass].SetValue("server secret")

	bar, _ = bar.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	for _, r := range "work" {
		bar, _ = bar.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	bar, cmd := bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !bar.profileSave.pending {
		t.Fatal("saving the named profile did not start a write")
	}
	result := cmd()
	request, ok := result.(profileWriteRequestMsg)
	if !ok {
		t.Fatalf("save command returned %T, want profileWriteRequestMsg", result)
	}
	if request.profile.Name != "work" || request.profile.Password != "server secret" {
		t.Errorf("save request profile = %#v", request.profile)
	}
}

func TestSavingExistingProfileRequiresOverwriteConfirmation(t *testing.T) {
	profile := config.Profile{Name: "work", Protocol: "FTP", Host: "old.example.org"}
	bar := NewConnectionBar().withProfilesLoaded(profilesLoadedMsg{profiles: []config.Profile{profile}})
	bar.selectedProfile = profile.Name
	bar.protocol = client.FTPS
	bar.inputs[fieldHost].SetValue("new.example.org")

	bar, _ = bar.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	bar, _ = bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !bar.profileSave.overwriting {
		t.Fatal("existing profile save did not request confirmation")
	}
	bar, cmd := bar.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("confirming overwrite returned no write command")
	}
	request := cmd().(profileWriteRequestMsg)
	if request.profile.Name != "work" || request.profile.Protocol != "FTPS" || request.profile.Host != "new.example.org" {
		t.Errorf("overwrite request = %#v", request.profile)
	}
}

func TestProfilePickerDeleteRequiresConfirmation(t *testing.T) {
	profile := config.Profile{Name: "work", Protocol: "FTP", Host: "ftp.example.org"}
	bar := NewConnectionBar().withProfilesLoaded(profilesLoadedMsg{profiles: []config.Profile{profile}})
	bar, _ = bar.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	bar, _ = bar.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !bar.profilePicker.confirmDelete {
		t.Fatal("delete key did not request confirmation")
	}
	bar, cmd := bar.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("confirming deletion returned no write command")
	}
	request := cmd().(profileWriteRequestMsg)
	if request.kind != profileWriteDelete || request.name != profile.Name {
		t.Errorf("delete request = %#v", request)
	}
}

func TestProfilePickerLoadsSelectedProfile(t *testing.T) {
	profile := config.Profile{
		Name:     "FTP backup",
		Protocol: "FTP",
		Host:     "ftp.example.org",
		User:     "backup",
		Password: "stored password",
	}
	bar := NewConnectionBar().withProfilesLoaded(profilesLoadedMsg{profiles: []config.Profile{profile}})
	bar, _ = bar.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if !bar.profilePicker.open {
		t.Fatal("Ctrl+P did not open the profile picker")
	}
	bar, _ = bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if bar.profilePicker.open {
		t.Error("profile picker stayed open after selecting a profile")
	}
	if bar.protocol != client.FTP || bar.inputs[fieldHost].Value() != profile.Host || bar.inputs[fieldPass].Value() != profile.Password {
		t.Errorf("profile settings were not restored: protocol=%s host=%q password=%q", bar.protocol, bar.inputs[fieldHost].Value(), bar.inputs[fieldPass].Value())
	}
}

func TestSavingAndDeletingProfilePersistsConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	profile := config.Profile{Name: "EMI", Protocol: "SFTP", Host: "sftp.example.org", User: "alice", Auth: config.AuthPassword, Password: "secret"}
	a := NewApp(nil, false, nil, "dev", false)
	a.connBar = a.connBar.withProfilesLoaded(profilesLoadedMsg{})

	_, saveCmd := a.handleProfileWrite(profileWriteRequestMsg{kind: profileWriteSave, profile: profile, name: profile.Name})
	saved, ok := saveCmd().(profileWriteDoneMsg)
	if !ok || saved.err != nil {
		t.Fatalf("save result = %#v, want successful profile write", saved)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Password != "secret" {
		t.Fatalf("saved profiles = %#v, want the password-bearing profile", loaded.Profiles)
	}
	a.connBar = a.connBar.withProfileWriteDone(saved)

	_, deleteCmd := a.handleProfileWrite(profileWriteRequestMsg{kind: profileWriteDelete, name: "EMI"})
	deleted := deleteCmd().(profileWriteDoneMsg)
	if deleted.err != nil {
		t.Fatalf("delete result error = %v", deleted.err)
	}
	loaded, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Profiles) != 0 {
		t.Errorf("profiles after deletion = %#v, want none", loaded.Profiles)
	}

	if _, err := os.Stat(filepath.Join(home, ".lazyftp", "config.json")); err != nil {
		t.Errorf("config file was not created: %v", err)
	}
}
