// ABOUTME: Configuration loading and FAL API key resolution.
// ABOUTME: Reads config.yaml and resolves the API key via env var, command, file, or .env.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type apiKeyConfig struct {
	Command string `yaml:"command"`
	File    string `yaml:"file"`
}

type config struct {
	Model          string                  `yaml:"model"`
	APIKeys        map[string]apiKeyConfig `yaml:"api-keys"`
	PreviewCommand string                  `yaml:"preview-command"`
}

// loadConfig reads config.yaml.
func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config.yaml: %w (expected config at %s)", err, path)
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config.yaml: %w", err)
	}

	if cfg.Model == "" {
		return nil, fmt.Errorf("config.yaml: 'model' field is required")
	}

	return &cfg, nil
}

// configDir returns the directory containing .env and config.yaml.
// It checks the binary directory first (development), then falls back
// to ~/.config/pix/ (installed via make install).
func configDir(binDir string) string {
	if hasConfigFiles(binDir) {
		return binDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return binDir
	}
	candidate := filepath.Join(home, ".config", "pix")
	if hasConfigFiles(candidate) {
		return candidate
	}
	// Fall back to binDir so error messages reference the expected location.
	return binDir
}

// hasConfigFiles returns true if the directory contains config.yaml or .env.
func hasConfigFiles(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		return true
	}
	return false
}

// resolveFALKey resolves the FAL API key via priority chain:
// 1. FAL_KEY environment variable
// 2. api-keys.fal.command (run command, use stdout)
// 3. api-keys.fal.file (read key from file)
// 4. .env fallback in config directory
func resolveFALKey(cfg *config, confDir string) (string, error) {
	if key := os.Getenv("FAL_KEY"); key != "" {
		return key, nil
	}

	if falCfg, ok := cfg.APIKeys["fal"]; ok {
		if falCfg.Command != "" {
			// Trust boundary: command is from user-owned config.yaml, not external input.
			out, err := exec.Command("sh", "-c", falCfg.Command).Output()
			if err != nil {
				return "", fmt.Errorf("api-keys.fal.command failed: %w", err)
			}
			key := strings.TrimSpace(string(out))
			if key != "" {
				return key, nil
			}
		}

		if falCfg.File != "" {
			data, err := os.ReadFile(falCfg.File)
			if err != nil {
				return "", fmt.Errorf("api-keys.fal.file: %w", err)
			}
			key := strings.TrimSpace(string(data))
			if key != "" {
				return key, nil
			}
		}
	}

	return loadFALKey(filepath.Join(confDir, ".env"))
}

// loadFALKey reads FAL_KEY from a .env file.
func loadFALKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read .env: %w (expected FAL_KEY in %s)", err, path)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) == "FAL_KEY" {
			v := strings.TrimSpace(value)
			if v != "" {
				return v, nil
			}
		}
	}

	return "", fmt.Errorf("FAL_KEY not found in %s", path)
}

// resolveConfDir runs os.Executable() + EvalSymlinks() + Dir() + configDir() and
// returns the directory where config files are expected.
func resolveConfDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks: %w", err)
	}
	return configDir(filepath.Dir(exePath)), nil
}
