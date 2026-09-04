package agent

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/logger"
)

func TestParserIncrementalAndDedup(t *testing.T) {
	p := NewParser(logger.Discard())
	var events []Event
	emit := func(ev Event) { events = append(events, ev) }

	p.HandleLine(`{"type":"system","subtype":"init","model":"gpt-5.2"}`, emit)
	p.HandleLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hel"}]}}`, emit)
	p.HandleLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}`, emit)
	p.HandleLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}`, emit)
	p.HandleLine(`{"type":"result","result":"Hello"}`, emit)

	if !p.GotResult {
		t.Fatal("expected result")
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != EventDelta || events[0].Text != "Hel" {
		t.Fatalf("first delta = %#v", events[0])
	}
	if events[1].Type != EventDelta || events[1].Text != "lo" {
		t.Fatalf("second delta = %#v", events[1])
	}
	if events[2].Type != EventResult || events[2].Text != "Hello" || events[2].Model != "gpt-5.2" {
		t.Fatalf("result = %#v", events[2])
	}
}

func TestParserToolCallResetsBuffer(t *testing.T) {
	p := NewParser(logger.Discard())
	var texts []string
	emit := func(ev Event) {
		if ev.Type == EventDelta {
			texts = append(texts, ev.Text)
		}
	}
	p.HandleLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"A"}]}}`, emit)
	p.HandleLine(`{"type":"tool_call","subtype":"started"}`, emit)
	p.HandleLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"B"}]}}`, emit)
	if strings.Join(texts, "") != "AB" {
		t.Fatalf("texts = %v", texts)
	}
}

func TestParserIgnoresNonJSON(t *testing.T) {
	p := NewParser(logger.Discard())
	called := false
	p.HandleLine("not-json", func(Event) { called = true })
	if called {
		t.Fatal("should ignore raw lines")
	}
}

func TestBuildArgs(t *testing.T) {
	args := buildArgs(Options{Model: "auto"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--output-format stream-json") {
		t.Fatalf("%v", args)
	}
	foundAuto := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "auto" {
			foundAuto = true
		}
	}
	if !foundAuto {
		t.Fatalf("auto should pass --model auto, got %v", args)
	}
	args = buildArgs(Options{Model: "gpt-5.2"})
	found := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "gpt-5.2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%v", args)
	}
}

func TestMergeEnvReplacesKey(t *testing.T) {
	env := mergeEnv(Options{
		APIKey: "new-key",
		Env:    []string{"FOO=bar", "CURSOR_API_KEY=old"},
	})
	hasNew, hasOld := false, false
	for _, kv := range env {
		if kv == "CURSOR_API_KEY=new-key" {
			hasNew = true
		}
		if kv == "CURSOR_API_KEY=old" {
			hasOld = true
		}
	}
	if !hasNew || hasOld {
		t.Fatalf("%v", env)
	}
}

func TestCLIRunnerWithFakeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake agent")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	script := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"echo '{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"test-model\"}'\n" +
		"echo '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"Hi\"}]}}'\n" +
		"echo '{\"type\":\"result\",\"result\":\"Hi\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(logger.Discard())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := r.Run(ctx, "hello", Options{Bin: bin, Model: "auto"})
	var got []Event
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) == 0 {
		t.Fatal("no events")
	}
	var result Event
	for _, ev := range got {
		if ev.Type == EventError {
			t.Fatalf("error event: %v", ev.Err)
		}
		if ev.Type == EventResult {
			result = ev
		}
	}
	if result.Text != "Hi" || result.Model != "test-model" {
		t.Fatalf("result = %#v events=%#v", result, got)
	}
}

func TestVerifyFakeVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'agent 1.2.3'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := Verify(context.Background(), bin, exec.CommandContext)
	if err != nil {
		t.Fatal(err)
	}
	if v != "agent 1.2.3" {
		t.Fatalf("version = %q", v)
	}
}

func TestConsume(t *testing.T) {
	p := NewParser(logger.Discard())
	input := strings.NewReader("{\"type\":\"result\",\"result\":\"ok\"}\n")
	var got Event
	if err := p.Consume(input, func(ev Event) { got = ev }); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got.Type != EventResult || got.Text != "ok" {
		t.Fatalf("%#v", got)
	}
}
