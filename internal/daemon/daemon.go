// Package daemon manages a background PID-file process.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

type Manager struct {
	StateDir string
	LogFile  string
	PIDFile  string
}

func New(cfg config.Config) Manager {
	state := cfg.StateDir
	if state == "" {
		state = config.StateDir()
	}
	logFile := cfg.LogPath()
	return Manager{
		StateDir: state,
		LogFile:  logFile,
		PIDFile:  config.PIDFile(state),
	}
}

func (m Manager) EnsureDir() error {
	return os.MkdirAll(m.StateDir, 0o700)
}

func (m Manager) ReadPID() (int, bool) {
	raw, err := os.ReadFile(m.PIDFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func (m Manager) WritePID(pid int) error {
	if err := m.EnsureDir(); err != nil {
		return err
	}
	return os.WriteFile(m.PIDFile, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func (m Manager) ClearPID() {
	_ = os.Remove(m.PIDFile)
}

func (m Manager) RunningPID() (int, bool) {
	pid, ok := m.ReadPID()
	if !ok {
		return 0, false
	}
	if !Alive(pid) {
		m.ClearPID()
		return 0, false
	}
	return pid, true
}

func (m Manager) RegisterForeground() error {
	return m.WritePID(os.Getpid())
}

type Status struct {
	Running bool
	PID     int
	LogFile string
}

func (m Manager) Status() Status {
	pid, ok := m.RunningPID()
	return Status{Running: ok, PID: pid, LogFile: m.LogFile}
}

func (m Manager) Stop() (int, error) {
	pid, ok := m.RunningPID()
	if !ok {
		m.ClearPID()
		return 0, ErrNotRunning
	}
	if err := Terminate(pid); err != nil {
		m.ClearPID()
		return pid, fmt.Errorf("signal process %d: %w", pid, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			m.ClearPID()
			return pid, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = Kill(pid)
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	m.ClearPID()
	return pid, nil
}

func (m Manager) Start(exe string, args []string, env []string) (int, error) {
	if pid, ok := m.RunningPID(); ok {
		return pid, fmt.Errorf("already running (pid %d); use restart", pid)
	}
	m.ClearPID()
	if err := m.EnsureDir(); err != nil {
		return 0, err
	}

	logFile, err := os.OpenFile(m.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if env != nil {
		cmd.Env = env
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	pid := cmd.Process.Pid
	if err := m.WritePID(pid); err != nil {
		_ = Terminate(pid)
		return 0, err
	}
	go func() { _ = cmd.Wait() }()

	time.Sleep(200 * time.Millisecond)
	if !Alive(pid) {
		m.ClearPID()
		return 0, fmt.Errorf("process exited immediately; see %s", m.LogFile)
	}
	return pid, nil
}

var ErrNotRunning = fmt.Errorf("not running")
