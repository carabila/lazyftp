package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/carabila/lazyftp/internal/client"
)

func keyMsg(s string) tea.KeyPressMsg {
	if s == " " {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

var (
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	left     = tea.KeyPressMsg{Code: tea.KeyLeft}
	right    = tea.KeyPressMsg{Code: tea.KeyRight}
)

// The FTP form follows its visual order (Protocol, Host, Port, User, Pass).
func TestTabFollowsVisualOrder(t *testing.T) {
	bar := NewConnectionBar()
	want := []connField{fieldHost, fieldPort, fieldUser, fieldPass, fieldProtocol}

	bar, _ = bar.Update(tab) // off the protocol field, onto Host
	for i, field := range want {
		if bar.focused != field {
			t.Fatalf("step %d: focused field %d, want %d", i, bar.focused, field)
		}
		bar, _ = bar.Update(tab)
	}
}

// The protocol field has no text input behind it, so tabbing onto it used to
// focus an uninitialised one and panic.
func TestTabbingReachesEveryFieldAndWrapsAround(t *testing.T) {
	bar := NewConnectionBar()
	start := bar.focused

	for i := 0; i < len(bar.visibleFields()); i++ {
		bar, _ = bar.Update(tab)
	}
	if bar.focused != start {
		t.Errorf("a full cycle of tab ended on field %d, want %d", bar.focused, start)
	}

	for i := 0; i < len(bar.visibleFields()); i++ {
		bar, _ = bar.Update(shiftTab)
	}
	if bar.focused != start {
		t.Errorf("a full cycle of shift+tab ended on field %d, want %d", bar.focused, start)
	}
}

func TestProtocolCyclesAndPortPlaceholderFollows(t *testing.T) {
	bar := NewConnectionBar()
	if bar.focused != fieldProtocol {
		t.Fatalf("the bar opens on field %d, want the protocol field", bar.focused)
	}

	if got := bar.inputs[fieldPort].Placeholder; got != "21" {
		t.Errorf("port placeholder is %q on FTP, want \"21\"", got)
	}

	bar, _ = bar.Update(right)
	bar, _ = bar.Update(right)
	if bar.protocol != client.SFTP {
		t.Fatalf("two presses of right landed on %s, want SFTP", bar.protocol)
	}
	if got := bar.inputs[fieldPort].Placeholder; got != "22" {
		t.Errorf("port placeholder is %q on SFTP, want \"22\"", got)
	}

	bar, _ = bar.Update(left)
	if bar.protocol != client.FTPS {
		t.Errorf("left from SFTP landed on %s, want FTPS", bar.protocol)
	}
}

// Key.String() prints the space bar as "space", not literal " " -- a case
// that matched on " " silently stopped working when this migrated to v2.
func TestSpaceAlsoCyclesProtocol(t *testing.T) {
	bar := NewConnectionBar()
	start := bar.protocol

	bar, _ = bar.Update(keyMsg(" "))
	if bar.protocol == start {
		t.Errorf("space did not cycle the protocol from %s", start)
	}
}

// Cycling the protocol must not leak keystrokes into the text fields, and typing
// must not reach them while the protocol is focused.
func TestProtocolKeysDoNotReachTheInputs(t *testing.T) {
	bar := NewConnectionBar()
	bar, _ = bar.Update(keyMsg("x"))
	bar, _ = bar.Update(right)

	for f, in := range bar.inputs {
		if in.Value() != "" {
			t.Errorf("field %d holds %q, want it untouched", f, in.Value())
		}
	}

	bar, _ = bar.Update(tab)
	bar, _ = bar.Update(keyMsg("h"))
	if got := bar.inputs[fieldHost].Value(); got != "h" {
		t.Errorf("host holds %q after tabbing to it and typing, want \"h\"", got)
	}
}

// Port only accepts digits -- anything else typed must be swallowed, but
// editing keys (backspace here) still need to reach the input.
func TestPortOnlyAcceptsDigits(t *testing.T) {
	bar := NewConnectionBar()
	bar, _ = bar.Update(tab) // Host
	bar, _ = bar.Update(tab) // Port

	for _, s := range []string{"a", "!", " ", "2", "1"} {
		bar, _ = bar.Update(keyMsg(s))
	}
	if got := bar.inputs[fieldPort].Value(); got != "21" {
		t.Errorf("port holds %q, want \"21\" (letters/symbols/space rejected)", got)
	}

	backspace := tea.KeyPressMsg{Code: tea.KeyBackspace}
	bar, _ = bar.Update(backspace)
	if got := bar.inputs[fieldPort].Value(); got != "2" {
		t.Errorf("backspace should still edit the port field, got %q", got)
	}
}

func TestSFTPAuthModeChangesVisibleFields(t *testing.T) {
	bar := NewConnectionBar()
	bar.protocol = client.SFTP

	for _, field := range []connField{fieldHost, fieldPort, fieldUser, fieldAuth} {
		bar, _ = bar.Update(tab)
		if bar.focused != field {
			t.Fatalf("tab reached field %d, want %d", bar.focused, field)
		}
	}
	if bar.auth != client.SFTPAuthPassword {
		t.Fatalf("default SFTP authentication is %d, want password", bar.auth)
	}

	bar, _ = bar.Update(right)
	if bar.auth != client.SFTPAuthIdentityFile {
		t.Fatal("right on the authentication selector did not choose identity-file auth")
	}
	bar, _ = bar.Update(tab)
	if bar.focused != fieldIdentity {
		t.Fatalf("the first identity-mode field is %d, want identity path", bar.focused)
	}
	bar, _ = bar.Update(tab)
	if bar.focused != fieldKeyPassphrase {
		t.Fatalf("the next identity-mode field is %d, want key passphrase", bar.focused)
	}
}

func TestSubmittingIdentityModeIncludesKeyDetails(t *testing.T) {
	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthIdentityFile
	bar.inputs[fieldHost].SetValue("sftp.example.org")
	bar.inputs[fieldUser].SetValue("alice")
	bar.inputs[fieldIdentity].SetValue("~/.ssh/work")
	bar.inputs[fieldKeyPassphrase].SetValue("secret")

	_, cmd := bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting the form returned no command")
	}
	result := cmd()
	msg, ok := result.(ConnectMsg)
	if !ok {
		t.Fatalf("submit returned %T, want ConnectMsg", result)
	}
	if msg.Protocol != client.SFTP || msg.SFTPAuth != client.SFTPAuthIdentityFile {
		t.Errorf("submitted protocol/auth = %s/%d, want SFTP/identity-file", msg.Protocol, msg.SFTPAuth)
	}
	if msg.IdentityFile != "~/.ssh/work" || msg.KeyPassphrase != "secret" {
		t.Errorf("submitted identity details = %q/%q", msg.IdentityFile, msg.KeyPassphrase)
	}
}

func TestIdentityPickerOpensInSSHDirectory(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthIdentityFile
	bar.focused = fieldIdentity
	bar, cmd := bar.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if !bar.picker.open {
		t.Fatal("Ctrl+O did not open the identity picker")
	}
	if cmd == nil {
		t.Fatal("opening the picker returned no directory-loading command")
	}
	bar, _ = bar.Update(cmd())
	if len(bar.picker.entries) != 1 || bar.picker.entries[0].name != "id_ed25519" {
		t.Fatalf("picker entries = %#v, want the identity file", bar.picker.entries)
	}
}

func TestIdentityPickerFallsBackToHomeWhenSSHDirectoryIsMissing(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "identity"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthIdentityFile
	bar.focused = fieldIdentity
	bar, cmd := bar.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("opening the picker returned no directory-loading command")
	}
	bar, _ = bar.Update(cmd())
	if bar.picker.dir != home {
		t.Errorf("picker opened in %q, want fallback home %q", bar.picker.dir, home)
	}
	if len(bar.picker.entries) != 1 || bar.picker.entries[0].name != "identity" {
		t.Errorf("picker entries = %#v, want the home-directory identity", bar.picker.entries)
	}
}

func TestIdentityPickerSelectsAFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}

	picker := keyFilePicker{open: true, dir: dir, loading: true, seq: 1}
	loaded := picker.load()().(keyFilePickerLoadedMsg)
	picker = picker.withEntries(loaded)
	if len(picker.entries) != 1 || picker.entries[0].name != "id_ed25519" {
		t.Fatalf("picker entries = %#v, want the identity file", picker.entries)
	}

	bar := NewConnectionBar()
	bar.protocol = client.SFTP
	bar.auth = client.SFTPAuthIdentityFile
	bar.focused = fieldIdentity
	bar.picker = picker
	bar, _ = bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if got := bar.inputs[fieldIdentity].Value(); got != keyPath {
		t.Errorf("selected identity path = %q, want %q", got, keyPath)
	}
	if bar.picker.open {
		t.Error("picker stayed open after selecting a file")
	}
}
