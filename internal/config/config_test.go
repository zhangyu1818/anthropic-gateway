package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"anthropic-gateway/internal/config"
)

func TestLoadWithEnvExpansionAndDefaults(t *testing.T) {
	t.Setenv("UPSTREAM_KEY", "secret-123")
	cfgPath := writeTempConfig(t, `
model_list:
  - model_name: sonnet
    params:
      model: glm-4.7
      api_base: https://api.example.com
      api_key: ${UPSTREAM_KEY}
`)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Listen, ":4000"; got != want {
		t.Fatalf("listen = %q, want %q", got, want)
	}
	if got, want := cfg.ModelList[0].Params.AuthType, config.AuthTypeXAPIKey; got != want {
		t.Fatalf("auth_type = %q, want %q", got, want)
	}
	if got, want := cfg.ModelList[0].Params.APIKey, "secret-123"; got != want {
		t.Fatalf("api_key = %q, want %q", got, want)
	}
}

func TestLoadFailsOnDuplicateModel(t *testing.T) {
	cfgPath := writeTempConfig(t, `
model_list:
  - model_name: sonnet
    params:
      model: glm-4.7
      api_base: https://api.example.com
      api_key: a
  - model_name: sonnet
    params:
      model: glm-4.8
      api_base: https://api2.example.com
      api_key: b
`)

	_, err := config.Load(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "duplicate model_name") {
		t.Fatalf("expected duplicate model_name error, got %v", err)
	}
}

func TestLoadFailsOnInvalidAPIBase(t *testing.T) {
	cfgPath := writeTempConfig(t, `
model_list:
  - model_name: sonnet
    params:
      model: glm-4.7
      api_base: ftp://invalid
      api_key: a
`)

	_, err := config.Load(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "must use http/https") {
		t.Fatalf("expected invalid api_base scheme error, got %v", err)
	}
}

func TestLoadFailsOnEmptyAPIKey(t *testing.T) {
	cfgPath := writeTempConfig(t, `
model_list:
  - model_name: sonnet
    params:
      model: glm-4.7
      api_base: https://api.example.com
      api_key: ""
`)

	_, err := config.Load(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("expected api_key required error, got %v", err)
	}
}

func TestLoadFailsOnInvalidAuthType(t *testing.T) {
	cfgPath := writeTempConfig(t, `
model_list:
  - model_name: sonnet
    params:
      model: glm-4.7
      api_base: https://api.example.com
      api_key: a
      auth_type: token
`)

	_, err := config.Load(cfgPath)
	if err == nil || !strings.Contains(err.Error(), "auth_type") {
		t.Fatalf("expected auth_type error, got %v", err)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
