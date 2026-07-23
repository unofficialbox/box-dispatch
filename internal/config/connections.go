package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ConnectionSettings struct {
	SalesforceAlias   string `json:"salesforceAlias,omitempty"`
	DatabricksHost    string `json:"databricksHost,omitempty"`
	DatabricksProfile string `json:"databricksProfile,omitempty"`
	AWSProfile        string `json:"awsProfile,omitempty"`
	AWSRegion         string `json:"awsRegion,omitempty"`
}

func ConnectionSettingsPath() string {
	return filepath.Join(Paths().Root, ".windlass", "connections.json")
}

func LoadConnectionSettings() (ConnectionSettings, error) {
	settings := ConnectionSettings{}
	data, err := os.ReadFile(ConnectionSettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read connection settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("parse connection settings: %w", err)
	}
	return settings, nil
}

func SaveConnectionSettings(settings ConnectionSettings) error {
	path := ConnectionSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Windlass state directory: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode connection settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write connection settings: %w", err)
	}
	return os.Chmod(path, 0o600)
}
