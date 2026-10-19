package system

import (
	"path/filepath"
	"testing"
)

func TestXDGDirectoriesRespectEnvAndAreAbsolute(t *testing.T) {
	tmp := t.TempDir()
	cfgEnv := filepath.Join(tmp, "custom_config")
	dataEnv := filepath.Join(tmp, "custom_data")
	cacheEnv := filepath.Join(tmp, "custom_cache")

	t.Setenv("XDG_CONFIG_HOME", cfgEnv)
	t.Setenv("XDG_DATA_HOME", dataEnv)
	t.Setenv("XDG_CACHE_HOME", cacheEnv)

	if got, want := ConfigDir(), filepath.Join(cfgEnv, "tuisense"); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
	if got, want := ProfilesDir(), filepath.Join(cfgEnv, "tuisense", "profiles"); got != want {
		t.Fatalf("ProfilesDir() = %q, want %q", got, want)
	}
	if got, want := DataDir(), filepath.Join(dataEnv, "tuisense"); got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
	if got, want := CacheDir(), filepath.Join(cacheEnv, "tuisense"); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
}
