package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitOmitsSecrets(t *testing.T) {
	unit := SystemdUnit("/opt/curapi", "/home/u/.curapi/env.json", "/usr/bin")
	if strings.Contains(unit, "CURSOR_API_KEY") || strings.Contains(unit, "authz") {
		t.Fatal("secrets must not be inlined into the unit file")
	}
	if !strings.Contains(unit, "run --config") {
		t.Fatalf("missing config flag:\n%s", unit)
	}
	if !strings.Contains(unit, "/home/u/.curapi/env.json") {
		t.Fatal("missing env path")
	}
}

func TestLaunchdPlistEscapes(t *testing.T) {
	plist := LaunchdPlist("com.curapi", `/tmp/a&b`, `/tmp/c<d>`, "PATH", `/tmp/log`)
	if strings.Contains(plist, `<d>`) && !strings.Contains(plist, `&lt;`) {
		t.Fatal("unescaped xml")
	}
	if !strings.Contains(plist, "&amp;") {
		t.Fatalf("expected escape: %s", plist)
	}
}

func TestInstallRefreshLinux(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		BinDir:          filepath.Join(root, "bin"),
		Binary:          filepath.Join(root, "bin", Name),
		StateDir:        filepath.Join(root, "state"),
		EnvFile:         filepath.Join(root, "state", "env.json"),
		LogFile:         filepath.Join(root, "state", "server.log"),
		SystemdUserDir:  filepath.Join(root, "systemd"),
		SystemdUserUnit: filepath.Join(root, "systemd", Name+".service"),
	}
	var calls []string
	inst := &Installer{
		Paths:  paths,
		GOOS:   "linux",
		Home:   root,
		Path:   "/usr/bin",
		Stdout: ioDiscard{},
		Exec: func(name string, args ...string) (string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return "", nil
		},
	}

	if err := inst.Install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Binary); err != nil {
		t.Fatal("binary not copied")
	}
	if _, err := os.Stat(paths.EnvFile); err != nil {
		t.Fatal("env.json not created")
	}
	unit, err := os.ReadFile(paths.SystemdUserUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "ExecStart=") {
		t.Fatalf("%s", unit)
	}

	// Second install refreshes: disable then enable again.
	if err := inst.Reinstall(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "disable --now") {
		t.Fatalf("reinstall should stop existing unit, calls:\n%s", joined)
	}
	enableCount := strings.Count(joined, "enable --now")
	if enableCount < 2 {
		t.Fatalf("expected enable on install and reinstall, got %d\n%s", enableCount, joined)
	}

	if err := inst.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.SystemdUserUnit); !os.IsNotExist(err) {
		t.Fatal("unit should be removed")
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	inst := &Installer{GOOS: "plan9", Stdout: ioDiscard{}}
	if err := inst.installPlatform(); err == nil {
		t.Fatal("expected error")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
