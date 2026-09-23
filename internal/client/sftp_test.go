package client

import (
	"errors"
	"testing"

	"github.com/pkg/sftp"
)

func TestSFTPInitialDirReturnsServerWorkingDirectory(t *testing.T) {
	c := &SFTPClient{client: &sftp.Client{}, initialDir: "/home/alice"}

	got, err := c.InitialDir()
	if err != nil {
		t.Fatalf("InitialDir() error = %v", err)
	}
	if got != "/home/alice" {
		t.Errorf("InitialDir() = %q, want /home/alice", got)
	}
}

func TestSFTPInitialDirReturnsFallbackWithResolutionError(t *testing.T) {
	wantErr := errors.New("REALPATH unavailable")
	c := &SFTPClient{client: &sftp.Client{}, initialDir: "/", initialDirErr: wantErr}

	got, err := c.InitialDir()
	if got != "/" {
		t.Errorf("InitialDir() = %q, want fallback /", got)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("InitialDir() error = %v, want %v", err, wantErr)
	}
}
