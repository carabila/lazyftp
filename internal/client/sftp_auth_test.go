package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func writeTestIdentity(t *testing.T, passphrase string) string {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(privateKey, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSFTPAuthAcceptsUnencryptedIdentityFile(t *testing.T) {
	method, err := sftpAuth(ConnectionOptions{
		SFTPAuth:     SFTPAuthIdentityFile,
		IdentityFile: writeTestIdentity(t, ""),
	})
	if err != nil {
		t.Fatalf("sftpAuth() error = %v", err)
	}
	if method == nil {
		t.Fatal("sftpAuth() returned a nil authentication method")
	}
}

func TestSFTPAuthAcceptsPassphraseProtectedIdentityFile(t *testing.T) {
	method, err := sftpAuth(ConnectionOptions{
		SFTPAuth:      SFTPAuthIdentityFile,
		IdentityFile:  writeTestIdentity(t, "key passphrase"),
		KeyPassphrase: "key passphrase",
	})
	if err != nil {
		t.Fatalf("sftpAuth() error = %v", err)
	}
	if method == nil {
		t.Fatal("sftpAuth() returned a nil authentication method")
	}
}

func TestSFTPAuthExplainsMissingOrWrongKeyPassphrase(t *testing.T) {
	identity := writeTestIdentity(t, "key passphrase")

	_, err := sftpAuth(ConnectionOptions{SFTPAuth: SFTPAuthIdentityFile, IdentityFile: identity})
	if err == nil || !strings.Contains(err.Error(), "requires a passphrase") {
		t.Errorf("missing passphrase error = %v, want a clear passphrase message", err)
	}

	_, err = sftpAuth(ConnectionOptions{
		SFTPAuth:      SFTPAuthIdentityFile,
		IdentityFile:  identity,
		KeyPassphrase: "wrong",
	})
	if err == nil || !strings.Contains(err.Error(), "unable to parse identity file") {
		t.Errorf("wrong passphrase error = %v, want an identity-file parse error", err)
	}
}

func TestSFTPAuthRejectsMissingIdentityFile(t *testing.T) {
	_, err := sftpAuth(ConnectionOptions{SFTPAuth: SFTPAuthIdentityFile})
	if err == nil || !strings.Contains(err.Error(), "identity file path is empty") {
		t.Errorf("missing identity error = %v, want an empty-path message", err)
	}
}

func TestSFTPAuthReportsUnreadableIdentityFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	_, err := sftpAuth(ConnectionOptions{SFTPAuth: SFTPAuthIdentityFile, IdentityFile: path})
	if err == nil || !strings.Contains(err.Error(), "unable to read identity file") {
		t.Errorf("unreadable identity error = %v, want a file-read message", err)
	}
}

func TestSFTPAuthKeepsPasswordAuthenticationAvailable(t *testing.T) {
	method, err := sftpAuth(ConnectionOptions{SFTPAuth: SFTPAuthPassword, Password: "server password"})
	if err != nil {
		t.Fatalf("sftpAuth() error = %v", err)
	}
	if method == nil {
		t.Fatal("sftpAuth() returned a nil authentication method")
	}
}

func TestResolveIdentityPathExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := resolveIdentityPath("~/.ssh/id_ed25519")
	if err != nil {
		t.Fatalf("resolveIdentityPath() error = %v", err)
	}
	want := filepath.Join(home, ".ssh", "id_ed25519")
	if got != want {
		t.Errorf("resolveIdentityPath() = %q, want %q", got, want)
	}
}
