package store

import (
	"fmt"
	"os"
	"os/user"

	"go.yaml.in/yaml/v3"
)

// Config is ~/.bored/config.yaml.
type Config struct {
	IDPrefix       string `yaml:"id_prefix"`
	Actor          string `yaml:"actor"`
	AutoCommit     bool   `yaml:"auto_commit"`
	DoneWindowDays int    `yaml:"done_window_days"`
	Editor         string `yaml:"editor"`
}

// DefaultConfig fills in sensible values.
func DefaultConfig(prefix string) Config {
	if prefix == "" {
		prefix = "BRD"
	}
	actor := os.Getenv("USER")
	if actor == "" {
		if u, err := user.Current(); err == nil {
			actor = u.Username
		}
	}
	if actor == "" {
		actor = "me"
	}
	return Config{IDPrefix: prefix, Actor: actor, AutoCommit: true, DoneWindowDays: 7}
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := DefaultConfig("")
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.IDPrefix == "" {
		cfg.IDPrefix = "BRD"
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
