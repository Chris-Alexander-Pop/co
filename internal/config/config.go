package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	dirName  = "co"
	fileName = "config.yaml"
)

type Config struct {
	URL      string `yaml:"url"`
	Token    string `yaml:"token"`
	Fallback string `yaml:"fallback"`
	Jobs     int    `yaml:"jobs"`
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", dirName, fileName), nil
}

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Fallback == "" {
		c.Fallback = "local"
	}
	if c.Fallback != "local" && c.Fallback != "cancel" {
		return nil, fmt.Errorf("fallback must be local or cancel, got %q", c.Fallback)
	}
	if c.Jobs <= 0 {
		c.Jobs = 8
	}
	if c.URL == "" || c.Token == "" {
		return nil, fmt.Errorf("%s needs url and token", path)
	}
	return &c, nil
}
