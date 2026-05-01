package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirName  = ".msgcli"
	configFileName = "config.json"
)

var ErrConfigNotFound = errors.New("config not found")

// Config holds the application configuration
type Config struct {
	ClientID        string   `json:"client_id,omitempty"`
	DefaultAuthFlow AuthFlow `json:"default_auth_flow,omitempty"`
}

// GetConfigDir returns the path to the config directory
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// LoadConfig loads the configuration from disk
func LoadConfig() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w - run 'msgcli auth setup' first", ErrConfigNotFound)
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if strings.TrimSpace(config.DefaultAuthFlow.String()) == "" {
		config.DefaultAuthFlow = FlowLegacy
	} else {
		normalizedFlow, err := ParseAuthFlow(config.DefaultAuthFlow.String())
		if err != nil {
			return nil, fmt.Errorf("invalid default_auth_flow in config: %w", err)
		}
		config.DefaultAuthFlow = normalizedFlow
	}

	return &config, nil
}

// LoadConfigOptional loads config when present; returns nil,nil when not configured.
func LoadConfigOptional() (*Config, error) {
	config, err := LoadConfig()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return config, nil
}

// SaveConfig saves the configuration to disk
func SaveConfig(config *Config) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}
	if strings.TrimSpace(config.DefaultAuthFlow.String()) == "" {
		config.DefaultAuthFlow = FlowLegacy
	}
	if err := config.DefaultAuthFlow.Validate(); err != nil {
		return err
	}

	dir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	path, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}
