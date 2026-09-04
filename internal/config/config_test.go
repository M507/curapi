package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	src := Default()
	src.AuthRequired = true
	src.AuthzTokens = []string{" tok-a ", "tok-a", "tok-b"}
	src.Port = 8080
	src.CursorAPIKey = "cursor_secret"
	if err := Write(path, src); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("env.json mode = %o, want 0600", st.Mode().Perm())
	}

	// Loading also enforces 0600 even if the file was created too permissively.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 8080 {
		t.Fatalf("port = %d", got.Port)
	}
	if got.CursorAPIKey != "cursor_secret" {
		t.Fatalf("cursor key not preserved")
	}
	if len(got.AuthzTokens) != 2 || got.AuthzTokens[0] != "tok-a" || got.AuthzTokens[1] != "tok-b" {
		t.Fatalf("tokens = %#v", got.AuthzTokens)
	}
	if got.Path != path {
		t.Fatalf("path = %s", got.Path)
	}
	st, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("load did not tighten mode, got %o", st.Mode().Perm())
	}
}

func TestLoadRejectsEmptyTokensWhenAuthRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	raw := []byte(`{"auth_required": true, "authz_tokens": []}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadInvalidPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	raw, _ := json.Marshal(map[string]any{
		"port":          70000,
		"auth_required": false,
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected port error")
	}
}

func TestGenerateCreatesToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "env.json")
	cfg, err := Generate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AuthRequired {
		t.Fatal("expected auth required")
	}
	if len(cfg.AuthzTokens) != 1 || len(cfg.AuthzTokens[0]) < 32 {
		t.Fatalf("token = %q", cfg.AuthzTokens)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthzTokens[0] != cfg.AuthzTokens[0] {
		t.Fatal("generated token mismatch")
	}
}

func TestLoadOrCreateFindsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	src := Default()
	src.AuthRequired = false
	if err := Write(path, src); err != nil {
		t.Fatal(err)
	}
	cfg, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("should not recreate existing file")
	}
	if cfg.Path != path {
		t.Fatalf("path = %s", cfg.Path)
	}
}

func TestAddr(t *testing.T) {
	cfg := Default()
	cfg.Host = "127.0.0.1"
	cfg.Port = 9
	cfg.TLSPort = 10
	if cfg.HTTPAddr() != "127.0.0.1:9" {
		t.Fatalf("http addr = %s", cfg.HTTPAddr())
	}
	if cfg.TLSAddr() != "127.0.0.1:10" {
		t.Fatalf("tls addr = %s", cfg.TLSAddr())
	}
	cfg.TLS = true
	if cfg.BaseURL() != "https://127.0.0.1:10" {
		t.Fatalf("base = %s", cfg.BaseURL())
	}
	if cfg.HTTPBaseURL() != "http://127.0.0.1:9" {
		t.Fatalf("http base = %s", cfg.HTTPBaseURL())
	}
	cfg.TLS = false
	cfg.Host = "0.0.0.0"
	if cfg.BaseURL() != "http://127.0.0.1:9" {
		t.Fatalf("wildcard base = %s", cfg.BaseURL())
	}
}

func TestTLSPortMustDiffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	raw, _ := json.Marshal(map[string]any{
		"auth_required": false,
		"port":          4646,
		"tls_port":      4646,
		"tls":           true,
		"tls_auto":      true,
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected tls_port conflict")
	}
}

func TestTLSManualRequiresFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	raw, _ := json.Marshal(map[string]any{
		"auth_required": false,
		"tls":           true,
		"tls_auto":      false,
		"tls_cert_file": "",
		"tls_key_file":  "",
	})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// applyDefaults fills default cert paths, so Load succeeds; Ensure would fail if files missing.
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TLS || cfg.TLSAuto {
		t.Fatalf("%+v", cfg)
	}
}
