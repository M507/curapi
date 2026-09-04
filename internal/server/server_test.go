package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/agent"
	"github.com/tageecc/cursor-agent-api-proxy/internal/auth"
	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
	"github.com/tageecc/cursor-agent-api-proxy/internal/logger"
	"github.com/tageecc/cursor-agent-api-proxy/internal/openai"
)

type eventRunner struct {
	events []agent.Event
	prompt string
	opts   agent.Options
}

func (e *eventRunner) Run(_ context.Context, prompt string, opts agent.Options) <-chan agent.Event {
	e.prompt = prompt
	e.opts = opts
	ch := make(chan agent.Event, len(e.events))
	for _, ev := range e.events {
		ch <- ev
	}
	close(ch)
	return ch
}

func newTestServer(t *testing.T, required bool, events []agent.Event) (*Server, *eventRunner) {
	t.Helper()
	cfg := config.Default()
	cfg.AuthRequired = required
	cfg.AuthzTokens = []string{"test-token"}
	cfg.SkipCLICheck = true
	cfg.CursorAPIKey = "cursor-from-env"
	runner := &eventRunner{events: events}
	s := New(Options{
		Config: cfg,
		Log:    logger.Discard(),
		Auth:   auth.New(required, cfg.AuthzTokens),
		Runner: runner,
	})
	s.SetCLIVersion("agent-test")
	return s, runner
}

func TestHealthOpenWithoutAuth(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing request id")
	}
}

func TestModelsRequiresAuth(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var list openai.ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) == 0 {
		t.Fatal("empty model list")
	}
}

func TestPlaceholderTokenRejected(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer not-needed")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAuthDisabledAllowsAnonymous(t *testing.T) {
	s, _ := newTestServer(t, false, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestChatValidatesMessages(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"auto"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestChatPassesVariousModels(t *testing.T) {
	models := []struct {
		request string
		cli     string
	}{
		{"auto", "auto"},
		{"claude-opus-5-low", "claude-opus-5-low"},
		{"claude-opus-5-high", "claude-opus-5-high"},
		{"composer-2.5", "composer-2.5"},
		{"composer-1", "composer-2.5"},
		{"gpt-5.2", "gpt-5.2"},
		{"gemini-3-flash", "gemini-3-flash"},
		{"cursor-grok-4.6-high-fast", "cursor-grok-4.6-high-fast"},
		{"grok", "cursor-grok-4.6-high-fast"},
	}
	for _, tc := range models {
		t.Run(tc.request, func(t *testing.T) {
			s, runner := newTestServer(t, true, []agent.Event{
				{Type: agent.EventResult, Text: "ok", Model: tc.cli},
			})
			body := `{"model":"` + tc.request + `","messages":[{"role":"user","content":"Hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if runner.opts.Model != tc.cli {
				t.Fatalf("cli model %q, want %q", runner.opts.Model, tc.cli)
			}
		})
	}
}

func TestResponsesPassesVariousModels(t *testing.T) {
	models := []string{"auto", "claude-opus-5-low", "composer-2.5", "gpt-5.2", "gemini-3-flash"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			s, runner := newTestServer(t, true, []agent.Event{
				{Type: agent.EventResult, Text: "ok", Model: model},
			})
			body := `{"model":"` + model + `","input":"ping"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if runner.opts.Model != model {
				t.Fatalf("cli model %q, want %q", runner.opts.Model, model)
			}
		})
	}
}

func TestModelsListIncludesCurrentCLIModels(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var list openai.ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	want := []string{"auto", "claude-opus-5-low", "composer-2.5", "gpt-5.2", "gemini-3-flash"}
	have := map[string]bool{}
	for _, m := range list.Data {
		have[m.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("model list missing %s (got %d models)", id, len(list.Data))
		}
	}
}

func TestChatNonStream(t *testing.T) {
	s, runner := newTestServer(t, true, []agent.Event{
		{Type: agent.EventDelta, Text: "Hel"},
		{Type: agent.EventResult, Text: "Hello", Model: "gpt-5.2"},
	})
	body := `{"model":"auto","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp openai.ChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Text() != "Hello" {
		t.Fatalf("%+v", resp)
	}
	if resp.Model != "gpt-5.2" {
		t.Fatalf("model %s", resp.Model)
	}
	if runner.prompt != "Hi" {
		t.Fatalf("prompt %q", runner.prompt)
	}
	if runner.opts.APIKey != "cursor-from-env" {
		t.Fatalf("api key not forwarded from env.json")
	}
	if runner.opts.Model != "auto" {
		t.Fatalf("cli model %q, want auto", runner.opts.Model)
	}
}

func TestResponsesEndpoint(t *testing.T) {
	s, runner := newTestServer(t, true, []agent.Event{
		{Type: agent.EventResult, Text: "Hello from responses", Model: "composer-1"},
	})
	body := `{"model":"composer-1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp openai.ResponsesResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Object != "response" || resp.Status != "completed" {
		t.Fatalf("%+v", resp)
	}
	if resp.Output[0].Content[0].Text != "Hello from responses" {
		t.Fatalf("%+v", resp)
	}
	if runner.prompt != "hello" {
		t.Fatalf("prompt %q", runner.prompt)
	}
	if runner.opts.Model != "composer-2.5" {
		t.Fatalf("cli model %q", runner.opts.Model)
	}
}

func TestResponsesPassesChosenModel(t *testing.T) {
	s, runner := newTestServer(t, true, []agent.Event{
		{Type: agent.EventResult, Text: "ok", Model: "claude-opus-5-low"},
	})
	body := `{"model":"claude-opus-5-low","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if runner.opts.Model != "claude-opus-5-low" {
		t.Fatalf("cli model %q, want claude-opus-5-low", runner.opts.Model)
	}
}

func TestResponsesAutoStaysAuto(t *testing.T) {
	s, runner := newTestServer(t, true, []agent.Event{
		{Type: agent.EventResult, Text: "ok", Model: "auto"},
	})
	body := `{"model":"auto","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if runner.opts.Model != "auto" {
		t.Fatalf("cli model %q, want auto", runner.opts.Model)
	}
}

func TestModelsAlias(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestChatStream(t *testing.T) {
	s, _ := newTestServer(t, true, []agent.Event{
		{Type: agent.EventDelta, Text: "Hi", Model: "auto"},
		{Type: agent.EventResult, Text: "Hi", Model: "auto"},
	})
	body := `{"model":"auto","stream":true,"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type %s", ct)
	}
	raw := rec.Body.String()
	if !strings.Contains(raw, "data: ") || !strings.Contains(raw, "[DONE]") {
		t.Fatalf("sse body %s", raw)
	}
	if !strings.Contains(raw, `"content":"Hi"`) {
		t.Fatalf("missing delta %s", raw)
	}
}

func TestNotFound(t *testing.T) {
	s, _ := newTestServer(t, false, nil)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodOptions, "/v1/models", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("missing CORS")
	}
}

func TestChatAgentError(t *testing.T) {
	s, _ := newTestServer(t, true, []agent.Event{
		{Type: agent.EventError, Err: errors.New("boom")},
	})
	body := `{"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidJSON(t *testing.T) {
	s, _ := newTestServer(t, true, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHTTPSListen(t *testing.T) {
	dir := t.TempDir()
	var (
		s     *Server
		cfg   config.Config
		errCh chan error
	)
	for attempt := 0; attempt < 8; attempt++ {
		httpLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		tlsLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		httpPort := httpLn.Addr().(*net.TCPAddr).Port
		tlsPort := tlsLn.Addr().(*net.TCPAddr).Port
		_ = httpLn.Close()
		_ = tlsLn.Close()

		cfg = config.Default()
		cfg.Host = "127.0.0.1"
		cfg.Port = httpPort
		cfg.TLSPort = tlsPort
		cfg.AuthRequired = false
		cfg.TLS = true
		cfg.TLSAuto = true
		cfg.StateDir = dir
		cfg.TLSCertFile = filepath.Join(dir, "cert.pem")
		cfg.TLSKeyFile = filepath.Join(dir, "key.pem")
		cfg.SkipCLICheck = true

		s = New(Options{
			Config: cfg,
			Log:    logger.Discard(),
			Auth:   auth.New(false, nil),
		})
		errCh = make(chan error, 1)
		go func() { errCh <- s.Start() }()
		select {
		case err := <-errCh:
			if err == nil || !strings.Contains(err.Error(), "address already in use") {
				t.Fatalf("server exited: %v", err)
			}
			continue
		case <-time.After(40 * time.Millisecond):
		}
		break
	}
	if s == nil {
		t.Fatal("could not bind test ports")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})

	insecure := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	plain := &http.Client{Timeout: 3 * time.Second}

	waitOK := func(client *http.Client, url string) {
		t.Helper()
		var last error
		for i := 0; i < 40; i++ {
			select {
			case err := <-errCh:
				t.Fatalf("server exited: %v", err)
			default:
			}
			resp, err := client.Get(url)
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("%s status %d body %s", url, resp.StatusCode, body)
				}
				return
			}
			last = err
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("%s failed: %v", url, last)
	}

	waitOK(plain, cfg.HTTPBaseURL()+"/health")
	waitOK(insecure, cfg.TLSBaseURL()+"/health")
	if _, err := os.Stat(cfg.TLSCertFile); err != nil {
		t.Fatal("cert not written")
	}
}
