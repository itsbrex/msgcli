package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("want error when config missing, got nil")
	}
	if !errors.Is(err, ErrConfigNotFound) {
		t.Errorf("error = %v, want ErrConfigNotFound in chain", err)
	}
}

func TestLoadConfigOptional_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := LoadConfigOptional()
	if err != nil {
		t.Fatalf("LoadConfigOptional returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("config = %+v, want nil when not configured", cfg)
	}
}

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := &Config{
		ClientID:        "11111111-2222-3333-4444-555555555555",
		DefaultAuthFlow: FlowMSALOffice,
	}
	if err := SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if got.ClientID != want.ClientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, want.ClientID)
	}
	if got.DefaultAuthFlow != want.DefaultAuthFlow {
		t.Errorf("DefaultAuthFlow = %q, want %q", got.DefaultAuthFlow, want.DefaultAuthFlow)
	}
}

func TestSaveConfig_DefaultsToLegacyFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := &Config{ClientID: "abc"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	if cfg.DefaultAuthFlow != FlowLegacy {
		t.Errorf("DefaultAuthFlow = %q, want %q", cfg.DefaultAuthFlow, FlowLegacy)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.DefaultAuthFlow != FlowLegacy {
		t.Errorf("loaded DefaultAuthFlow = %q, want %q", loaded.DefaultAuthFlow, FlowLegacy)
	}
}

func TestSaveConfig_NilReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveConfig(nil); err == nil {
		t.Fatal("want error for nil config, got nil")
	}
}

func TestSaveConfig_FilePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := SaveConfig(&Config{ClientID: "x"}); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Join(home, configDirName))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Errorf("config dir perm = %#o, want 0700", got)
	}

	fileInfo, err := os.Stat(filepath.Join(home, configDirName, configFileName))
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Errorf("config file perm = %#o, want 0600", got)
	}
}
