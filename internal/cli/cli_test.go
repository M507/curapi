package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

func withIO(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	origOut, origErr := Stdout, Stderr
	Stdout = &out
	Stderr = &out
	t.Cleanup(func() {
		Stdout = origOut
		Stderr = origErr
	})
	return &out
}

func TestParseArgs(t *testing.T) {
	opts, err := parseArgs([]string{"--config", "/tmp/env.json", "run", "8080"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.configPath != "/tmp/env.json" || opts.command != "run" || opts.port != 8080 {
		t.Fatalf("%+v", opts)
	}

	opts, err = parseArgs([]string{"start", "--port=9090", "--daemon-child"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.port != 9090 || !opts.daemonChild || opts.command != "start" {
		t.Fatalf("%+v", opts)
	}

	opts, err = parseArgs([]string{"--help"})
	if err != nil || !opts.help {
		t.Fatalf("%+v %v", opts, err)
	}
}

func TestParseArgsErrors(t *testing.T) {
	if _, err := parseArgs([]string{"--config"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseArgs([]string{"--port", "nope"}); err == nil {
		t.Fatal("expected port error")
	}
	if _, err := parseArgs([]string{"--unknown"}); err == nil {
		t.Fatal("expected unknown flag")
	}
	if _, err := parseArgs([]string{"run", "x"}); err == nil {
		t.Fatal("expected unexpected arg")
	}
}

func TestHelpAndVersion(t *testing.T) {
	out := withIO(t)
	if code := Execute([]string{"curapi", "--help"}); code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(out.String(), "reinstall") {
		t.Fatalf("help missing reinstall: %s", out.String())
	}
	out.Reset()
	if code := Execute([]string{"curapi", "version"}); code != 0 {
		t.Fatalf("version exit %d", code)
	}
	if !strings.Contains(out.String(), Version) {
		t.Fatalf("%s", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	out := withIO(t)
	if code := Execute([]string{"curapi", "nope"}); code != 1 {
		t.Fatalf("exit %d output %s", code, out.String())
	}
}

func TestBarePortIsStart(t *testing.T) {
	opts, err := parseArgs([]string{"4647"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != "4647" {
		t.Fatalf("%+v", opts)
	}
	if !isPort(opts.command) {
		t.Fatal("should detect port")
	}
}

func TestStatusWithoutServer(t *testing.T) {
	out := withIO(t)
	dir := t.TempDir()
	env := dir + "/env.json"
	src := config.Default()
	src.AuthRequired = false
	src.StateDir = dir
	if err := config.Write(env, src); err != nil {
		t.Fatal(err)
	}
	code := Execute([]string{"curapi", "--config", env, "status"})
	if code != 0 {
		t.Fatalf("exit %d output %s", code, out.String())
	}
	if !strings.Contains(out.String(), "not running") {
		t.Fatalf("output %s", out.String())
	}
}
