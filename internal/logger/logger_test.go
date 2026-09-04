package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

func TestSetupWritesFileAndRedacts(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.LogLevel = "debug"
	cfg.AuthRequired = false

	var buf bytes.Buffer
	res, err := Setup(cfg, &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Closer()

	res.Logger.Info("login", slog.String("authorization", "Bearer secret"), slog.String("path", "/v1"))
	if strings.Contains(buf.String(), "Bearer secret") {
		t.Fatalf("secret leaked: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "[redacted]") {
		t.Fatalf("expected redaction, got %s", buf.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "login") {
		t.Fatalf("log file missing entry: %s", raw)
	}
}

func TestParseLevel(t *testing.T) {
	if parseLevel("debug") != slog.LevelDebug {
		t.Fatal("debug")
	}
	if parseLevel("warning") != slog.LevelWarn {
		t.Fatal("warning")
	}
	if parseLevel("error") != slog.LevelError {
		t.Fatal("error")
	}
	if parseLevel("info") != slog.LevelInfo {
		t.Fatal("info")
	}
}
