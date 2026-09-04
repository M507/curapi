package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

func TestPIDLifecycle(t *testing.T) {
	dir := t.TempDir()
	m := New(config.Config{StateDir: dir, LogFile: filepath.Join(dir, "server.log")})
	if _, ok := m.RunningPID(); ok {
		t.Fatal("expected not running")
	}
	if err := m.WritePID(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	pid, ok := m.RunningPID()
	if !ok || pid != os.Getpid() {
		t.Fatalf("pid=%d ok=%v", pid, ok)
	}
	st := m.Status()
	if !st.Running {
		t.Fatal("status should be running")
	}
	m.ClearPID()
	if _, ok := m.RunningPID(); ok {
		t.Fatal("cleared pid still running")
	}
}

func TestAliveSelf(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("current process should be alive")
	}
	if Alive(1<<30) && runtime.GOOS != "windows" {
		// extremely high pid is almost certainly dead on unix
	}
}

func TestStartAndStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep binary differs")
	}
	dir := t.TempDir()
	m := New(config.Config{StateDir: dir, LogFile: filepath.Join(dir, "server.log")})
	pid, err := m.Start("sleep", []string{"30"}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if !Alive(pid) {
		t.Fatal("child not alive")
	}
	stopped, err := m.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if stopped != pid {
		t.Fatalf("stopped %d want %d", stopped, pid)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && Alive(pid) {
		time.Sleep(20 * time.Millisecond)
	}
	if Alive(pid) {
		t.Fatal("child still alive after stop")
	}
}

func TestStopNotRunning(t *testing.T) {
	m := New(config.Config{StateDir: t.TempDir()})
	if _, err := m.Stop(); err != ErrNotRunning {
		t.Fatalf("got %v", err)
	}
}
