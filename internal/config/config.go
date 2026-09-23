package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CurrentVersion is the schema version written to config.json.
const CurrentVersion = 1

const (
	AuthPassword     = "password"
	AuthIdentityFile = "identity_file"
)

// Profile is a named connection. Password is intentionally stored in plaintext
// when supplied; key passphrases are not part of the profile format.
type Profile struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Port         int    `json:"port,omitempty"`
	User         string `json:"user"`
	Auth         string `json:"auth,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
}

// Config is the versioned profile document stored in the user's lazyftp directory.
type Config struct {
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
}

// ConfigPath returns the per-user JSON profile file path.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %w", err)
	}
	return filepath.Join(home, ".lazyftp", "config.json"), nil
}

// Load reads the profile file and restricts its permissions before reading stored passwords.
func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	return load(path)
}

// Save atomically writes profiles with owner-only POSIX permissions.
func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return save(path, cfg)
}

func load(path string) (Config, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Config{Version: CurrentVersion, Profiles: []Profile{}}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("unable to inspect config file %s: %w", path, err)
	}
	if info.IsDir() {
		return Config{}, fmt.Errorf("config path %s is a directory", path)
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return Config{}, fmt.Errorf("unable to restrict config directory permissions: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return Config{}, fmt.Errorf("unable to restrict config file permissions: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("unable to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("unable to parse config file %s: %w", path, err)
	}
	if cfg.Version != CurrentVersion {
		return Config{}, fmt.Errorf("unsupported config version %d in %s", cfg.Version, path)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = []Profile{}
	}
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config file %s: %w", path, err)
	}
	return cfg, nil
}

func save(path string, cfg Config) error {
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = []Profile{}
	}
	if err := validate(cfg); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("unable to create config directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("unable to restrict config directory permissions: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to encode config: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("unable to create temporary config file in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("unable to restrict config file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("unable to write config file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("unable to sync config file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("unable to close config file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("unable to replace config file %s: %w", path, err)
	}
	return nil
}

func validate(cfg Config) error {
	seen := make([]string, 0, len(cfg.Profiles))
	for i, profile := range cfg.Profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			return fmt.Errorf("profile %d has an empty name", i+1)
		}
		for _, previous := range seen {
			if strings.EqualFold(previous, name) {
				return fmt.Errorf("duplicate profile name %q", name)
			}
		}
		seen = append(seen, name)
		if profile.Protocol != "FTP" && profile.Protocol != "FTPS" && profile.Protocol != "SFTP" {
			return fmt.Errorf("profile %q has unsupported protocol %q", name, profile.Protocol)
		}
		if strings.TrimSpace(profile.Host) == "" {
			return fmt.Errorf("profile %q has no host", name)
		}
		if profile.Port < 0 || profile.Port > 65535 {
			return fmt.Errorf("profile %q has invalid port %d", name, profile.Port)
		}
		if profile.Auth != "" && profile.Auth != AuthPassword && profile.Auth != AuthIdentityFile {
			return fmt.Errorf("profile %q has unsupported authentication method %q", name, profile.Auth)
		}
		if profile.Protocol != "SFTP" && profile.Auth == AuthIdentityFile {
			return fmt.Errorf("profile %q uses identity-file auth with %s", name, profile.Protocol)
		}
		if profile.Auth == AuthIdentityFile && strings.TrimSpace(profile.IdentityFile) == "" {
			return fmt.Errorf("profile %q has no identity file", name)
		}
	}
	return nil
}
