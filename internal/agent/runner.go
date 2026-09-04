// Package agent runs the Cursor CLI (`agent`) and turns stream-json into events.
package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/cursorcli"
)

const DefaultTimeout = 5 * time.Minute

type EventType int

const (
	EventDelta EventType = iota
	EventResult
	EventError
)

type Event struct {
	Type  EventType
	Text  string
	Model string
	Err   error
}

type Options struct {
	Model   string
	APIKey  string
	Bin     string
	CWD     string
	Timeout time.Duration
	Env     []string
}

type Runner interface {
	Run(ctx context.Context, prompt string, opts Options) <-chan Event
}

// CLIRunner spawns the Cursor agent CLI.
type CLIRunner struct {
	Log *slog.Logger
	// LookPath is used to resolve the agent binary. Tests may override it.
	LookPath func(string) (string, error)
	// Command builds the process. Tests may override it.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func NewRunner(log *slog.Logger) *CLIRunner {
	if log == nil {
		log = slog.Default()
	}
	return &CLIRunner{
		Log:      log,
		LookPath: exec.LookPath,
		Command:  exec.CommandContext,
	}
}

func (r *CLIRunner) Run(ctx context.Context, prompt string, opts Options) <-chan Event {
	out := make(chan Event, 16)
	go r.execute(ctx, prompt, opts, out)
	return out
}

func (r *CLIRunner) execute(ctx context.Context, prompt string, opts Options, out chan Event) {
	defer close(out)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := opts.Bin
	if bin == "" {
		bin = "agent"
	}
	if r.LookPath != nil {
		if resolved, err := r.LookPath(bin); err == nil {
			bin = resolved
		} else if !errors.Is(err, exec.ErrNotFound) {
			r.emit(ctx, out, Event{Type: EventError, Err: err})
			return
		}
	}

	args := buildArgs(opts)
	cmd := r.Command(ctx, bin, args...)
	if opts.CWD != "" {
		cmd.Dir = opts.CWD
	}
	cmd.Env = mergeEnv(opts)
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.emit(ctx, out, Event{Type: EventError, Err: err})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.emit(ctx, out, Event{Type: EventError, Err: err})
		return
	}

	if err := cmd.Start(); err != nil {
		r.emit(ctx, out, Event{Type: EventError, Err: notFoundError(err)})
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.drainStderr(stderr)
	}()

	parser := NewParser(r.Log)
	if err := parser.Consume(stdout, func(ev Event) {
		r.emit(ctx, out, ev)
	}); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		r.Log.Debug("agent stdout parse", "err", err)
	}

	waitErr := cmd.Wait()
	wg.Wait()

	if ctx.Err() == context.DeadlineExceeded {
		r.emit(ctx, out, Event{Type: EventError, Err: fmt.Errorf("request timed out after %s", timeout)})
		return
	}
	if waitErr != nil && ctx.Err() == nil && !parser.GotResult {
		r.emit(ctx, out, Event{Type: EventError, Err: fmt.Errorf("agent exited: %w", waitErr)})
	}
}

func (r *CLIRunner) emit(ctx context.Context, out chan Event, ev Event) {
	select {
	case <-ctx.Done():
		if ev.Type == EventError {
			select {
			case out <- ev:
			default:
			}
		}
	case out <- ev:
	}
}

func (r *CLIRunner) drainStderr(stderr io.Reader) {
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if len(line) > 500 {
			line = line[:500]
		}
		r.Log.Warn("agent stderr", "line", line)
	}
}

func buildArgs(opts Options) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--stream-partial-output",
		"--yolo",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

func mergeEnv(opts Options) []string {
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	if opts.APIKey == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "CURSOR_API_KEY=") {
			out = append(out, "CURSOR_API_KEY="+opts.APIKey)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, "CURSOR_API_KEY="+opts.APIKey)
	}
	return out
}

func notFoundError(err error) error {
	if errors.Is(err, exec.ErrNotFound) || isENOENT(err) {
		if runtime.GOOS == "windows" {
			return errors.New("Cursor CLI (agent) not found. Install: irm 'https://cursor.com/install?win32=true' | iex")
		}
		return errors.New("Cursor CLI (agent) not found. Install: curl https://cursor.com/install -fsS | bash")
	}
	return err
}

func isENOENT(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "executable file not found") ||
		strings.Contains(err.Error(), "ENOENT")
}

func Verify(ctx context.Context, bin string, command func(context.Context, string, ...string) *exec.Cmd) (version string, err error) {
	if bin == "" {
		bin = "agent"
	}
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(ctx, bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", notFoundError(err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", errors.New("Cursor CLI (agent) returned empty version")
	}
	return v, nil
}

// Parser converts stream-json NDJSON into delta/result events.
type Parser struct {
	Log        *slog.Logger
	turnBuffer string
	model      string
	GotResult  bool
}

func NewParser(log *slog.Logger) *Parser {
	if log == nil {
		log = slog.Default()
	}
	return &Parser{Log: log, model: "cursor-auto"}
}

func (p *Parser) Consume(r io.Reader, emit func(Event)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p.HandleLine(line, emit)
	}
	return sc.Err()
}

func (p *Parser) HandleLine(line string, emit func(Event)) {
	msg, err := cursorcli.ParseLine([]byte(line))
	if err != nil {
		p.Log.Debug("agent raw line", "line", truncate(line, 300))
		return
	}
	p.handle(msg, emit)
}

func (p *Parser) handle(msg cursorcli.Message, emit func(Event)) {
	switch {
	case msg.IsSystemInit():
		if msg.Model != "" {
			p.model = msg.Model
		}
	case msg.IsAssistant():
		text := msg.AssistantText()
		if text == "" {
			return
		}
		if text == p.turnBuffer {
			return
		}
		if strings.HasPrefix(text, p.turnBuffer) {
			diff := text[len(p.turnBuffer):]
			if diff != "" {
				emit(Event{Type: EventDelta, Text: diff, Model: p.model})
			}
			p.turnBuffer = text
			return
		}
		emit(Event{Type: EventDelta, Text: text, Model: p.model})
		p.turnBuffer += text
	case msg.IsToolCall():
		p.turnBuffer = ""
	case msg.IsResult():
		p.GotResult = true
		emit(Event{Type: EventResult, Text: msg.Result, Model: p.model})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
