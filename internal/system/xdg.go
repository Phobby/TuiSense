package system

import (
	"os"
	"path/filepath"
)

const appDirName = "tuisense"

// ConfigDir returns the XDG configuration directory for TuiSense
// ($XDG_CONFIG_HOME/tuisense or ~/.config/tuisense).
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, appDirName)
	}
	if cfgRoot, err := os.UserConfigDir(); err == nil && cfgRoot != "" {
		return filepath.Join(cfgRoot, appDirName)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", appDirName)
	}
	return filepath.Join(os.TempDir(), "tuisense-config")
}

// DataDir returns the XDG data directory for TuiSense
// ($XDG_DATA_HOME/tuisense or ~/.local/share/tuisense).
func DataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, appDirName)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", appDirName)
	}
	return filepath.Join(os.TempDir(), "tuisense-data")
}

// CacheDir returns the XDG cache directory for TuiSense
// ($XDG_CACHE_HOME/tuisense or ~/.cache/tuisense).
func CacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, appDirName)
	}
	if cacheRoot, err := os.UserCacheDir(); err == nil && cacheRoot != "" {
		return filepath.Join(cacheRoot, appDirName)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache", appDirName)
	}
	return filepath.Join(os.TempDir(), "tuisense-cache")
}

// ProfilesDir returns the default directory for storing controller profiles
// ($XDG_CONFIG_HOME/tuisense/profiles or ~/.config/tuisense/profiles).
func ProfilesDir() string {
	return filepath.Join(ConfigDir(), "profiles")
}
