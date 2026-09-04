// Package service installs, refreshes, and removes the OS auto-start unit.
package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

const (
	Name        = config.AppName
	DarwinLabel = "com." + config.AppName
	WindowsTask = "CurAPI"
)

// ExecRunner runs external commands (systemctl, launchctl, schtasks).
type ExecRunner func(name string, args ...string) (string, error)

type Paths struct {
	BinDir          string
	Binary          string
	StateDir        string
	EnvFile         string
	LogFile         string
	SystemdUserDir  string
	SystemdUserUnit string
	LaunchPlist     string
}

func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")
	state := config.StateDir()
	userSystemd := filepath.Join(home, ".config", "systemd", "user")
	launchDir := filepath.Join(home, "Library", "LaunchAgents")
	return Paths{
		BinDir:          binDir,
		Binary:          filepath.Join(binDir, Name),
		StateDir:        state,
		EnvFile:         filepath.Join(state, "env.json"),
		LogFile:         filepath.Join(state, "server.log"),
		SystemdUserDir:  userSystemd,
		SystemdUserUnit: filepath.Join(userSystemd, Name+".service"),
		LaunchPlist:     filepath.Join(launchDir, DarwinLabel+".plist"),
	}
}

type Installer struct {
	Paths  Paths
	Exec   ExecRunner
	Stdout io.Writer
	GOOS   string
	Home   string
	Path   string
}

func NewInstaller() *Installer {
	home, _ := os.UserHomeDir()
	return &Installer{
		Paths:  DefaultPaths(),
		Exec:   defaultExec,
		Stdout: os.Stdout,
		GOOS:   runtime.GOOS,
		Home:   home,
		Path:   os.Getenv("PATH"),
	}
}

func (i *Installer) printf(format string, args ...any) {
	if i.Stdout == nil {
		return
	}
	_, _ = fmt.Fprintf(i.Stdout, format, args...)
}

// Install copies the current binary into a stable location, writes the OS
// unit, and enables it. A second install refreshes the existing unit.
func (i *Installer) Install() error {
	return i.install(true)
}

func (i *Installer) Reinstall() error {
	i.printf("Reinstalling %s (refreshing existing installation)...\n", Name)
	return i.install(true)
}

func (i *Installer) Uninstall() error {
	i.printf("Uninstalling %s service...\n", Name)
	return i.uninstallPlatform()
}

func (i *Installer) install(refresh bool) error {
	if refresh {
		_ = i.uninstallQuiet()
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if err := os.MkdirAll(i.Paths.BinDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(i.Paths.StateDir, 0o700); err != nil {
		return err
	}
	if err := copyFile(exe, i.Paths.Binary, 0o755); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	if err := i.ensureEnvFile(); err != nil {
		return err
	}
	return i.installPlatform()
}

func (i *Installer) ensureEnvFile() error {
	if _, err := os.Stat(i.Paths.EnvFile); err == nil {
		return nil
	}
	_, err := config.Generate(i.Paths.EnvFile)
	if err != nil {
		return fmt.Errorf("create env.json: %w", err)
	}
	i.printf("Created config %s (mode 0600). Edit authz_tokens before exposing the API.\n", i.Paths.EnvFile)
	return nil
}

func (i *Installer) agentPATH() string {
	parts := []string{
		filepath.Join(i.Home, ".local", "bin"),
		filepath.Join(i.Home, ".cursor-agent", "bin"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/usr/bin",
		"/bin",
	}
	if i.Path != "" {
		parts = append(parts, i.Path)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func (i *Installer) runArgs() []string {
	return []string{"run", "--config", i.Paths.EnvFile}
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if sameFile(src, dst) {
		return os.Chmod(dst, mode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+Name+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func defaultExec(name string, args ...string) (string, error) {
	// implemented in exec.go to keep this file free of os/exec for tests that
	// inject Installer.Exec.
	return runCommand(name, args...)
}
