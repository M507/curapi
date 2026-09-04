// Package logger configures structured logging with secret redaction.
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

type SetupResult struct {
	Logger *slog.Logger
	Closer func() error
	File   string
}

func Setup(cfg config.Config, extra io.Writer) (SetupResult, error) {
	if err := cfg.EnsureStateDir(); err != nil {
		return SetupResult{}, err
	}
	logPath := cfg.LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return SetupResult{}, err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return SetupResult{}, err
	}

	writers := []io.Writer{os.Stderr, f}
	if extra != nil {
		writers = append(writers, extra)
	}
	w := io.MultiWriter(writers...)

	opts := &slog.HandlerOptions{
		Level:       parseLevel(cfg.LogLevel),
		ReplaceAttr: redactAttr,
	}
	if cfg.Debug {
		opts.Level = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	log := slog.New(handler)
	slog.SetDefault(log)
	return SetupResult{
		Logger: log,
		Closer: f.Close,
		File:   logPath,
	}, nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	switch strings.ToLower(a.Key) {
	case "token", "authorization", "api_key", "cursor_api_key", "authz_token",
		"password", "secret", "bearer":
		return slog.String(a.Key, "[redacted]")
	default:
		return a
	}
}

func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
