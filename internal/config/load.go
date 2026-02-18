package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ConfigFileName is the name of the Raven configuration file.
const ConfigFileName = "raven.toml"

// FindConfigFile walks up from the given directory to find raven.toml.
// Returns the absolute path to the config file, or an empty string if not found.
// Stops at the filesystem root.
func FindConfigFile(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	for {
		candidate := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			return "", nil
		}
		dir = parent
	}
}

// LoadFromFile parses the TOML file at the given path and returns the
// configuration and TOML metadata. The metadata can be used to detect
// unknown keys via MetaData.Undecoded().
func LoadFromFile(path string) (*Config, toml.MetaData, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, md, fmt.Errorf("loading config %s: %w", path, err)
	}
	return &cfg, md, nil
}
