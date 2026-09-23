package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadMissingConfigReturnsEmptyProfiles(t *testing.T) {
	cfg, err := load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.Version != CurrentVersion || len(cfg.Profiles) != 0 {
		t.Errorf("load() = %#v, want an empty version-%d config", cfg, CurrentVersion)
	}
}

func TestSaveAndLoadProfilesIncludingPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lazyftp", "config.json")
	want := Config{Profiles: []Profile{{
		Name:         "EMI",
		Protocol:     "SFTP",
		Host:         "sftp.example.org",
		Port:         2222,
		User:         "alice",
		Auth:         AuthPassword,
		Password:     "plain text password",
		IdentityFile: "",
	}}}

	if err := save(path, want); err != nil {
		t.Fatalf("save() error = %v", err)
	}
	got, err := load(path)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if got.Version != CurrentVersion || len(got.Profiles) != 1 {
		t.Fatalf("loaded config = %#v, want one versioned profile", got)
	}
	if got.Profiles[0] != (Profile{
		Name:     "EMI",
		Protocol: "SFTP",
		Host:     "sftp.example.org",
		Port:     2222,
		User:     "alice",
		Auth:     AuthPassword,
		Password: "plain text password",
	}) {
		t.Errorf("loaded profile = %#v, want saved profile", got.Profiles[0])
	}
}

func TestSaveOverwritesConfigAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := Config{Version: CurrentVersion, Profiles: []Profile{{Name: "first", Protocol: "FTP", Host: "one.example.org"}}}
	second := Config{Version: CurrentVersion, Profiles: []Profile{{Name: "second", Protocol: "FTPS", Host: "two.example.org"}}}

	if err := save(path, first); err != nil {
		t.Fatal(err)
	}
	if err := save(path, second); err != nil {
		t.Fatalf("second save() error = %v", err)
	}
	got, err := load(path)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].Name != "second" {
		t.Errorf("loaded profiles = %#v, want the replacement profile", got.Profiles)
	}
}

func TestSaveRestrictsConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are ACL-based")
	}

	path := filepath.Join(t.TempDir(), ".lazyftp", "config.json")
	if err := save(path, Config{Profiles: []Profile{{Name: "test", Protocol: "FTP", Host: "example.org"}}}); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		path string
		want os.FileMode
	}{{filepath.Dir(path), 0700}, {path, 0600}} {
		info, err := os.Stat(check.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != check.want {
			t.Errorf("permissions for %s = %04o, want %04o", check.path, got, check.want)
		}
	}
}

func TestLoadRestrictsPermissionsOfExistingConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are ACL-based")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	contents := `{"version":1,"profiles":[{"name":"test","protocol":"FTP","host":"example.org"}]}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err != nil {
		t.Fatalf("load() error = %v", err)
	}
	for _, check := range []struct {
		path string
		want os.FileMode
	}{{dir, 0700}, {path, 0600}} {
		info, err := os.Stat(check.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != check.want {
			t.Errorf("permissions for %s = %04o, want %04o", check.path, got, check.want)
		}
	}
}

func TestLoadRejectsUnsupportedVersionAndMalformedProfiles(t *testing.T) {
	for name, contents := range map[string]string{
		"version":  `{"version":2,"profiles":[]}`,
		"protocol": `{"version":1,"profiles":[{"name":"bad","protocol":"SSH"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := load(path); err == nil {
				t.Fatal("load() succeeded for invalid config")
			}
		})
	}
}

func TestConfigPathUsesLazyFTPUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	want := filepath.Join(home, ".lazyftp", "config.json")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadErrorDoesNotIncludeProfilePassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	secret := "not-for-errors"
	if err := os.WriteFile(path, []byte(`{"version":broken,"password":"`+secret+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := load(path)
	if err == nil {
		t.Fatal("load() succeeded for malformed JSON")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("config error exposed a password: %v", err)
	}
}
