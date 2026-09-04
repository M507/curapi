package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/agent"
	"github.com/tageecc/cursor-agent-api-proxy/internal/auth"
	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
	"github.com/tageecc/cursor-agent-api-proxy/internal/openai"
	"github.com/tageecc/cursor-agent-api-proxy/internal/tlsutil"
)

type Server struct {
	cfg     config.Config
	log     *slog.Logger
	auth    auth.Authenticator
	runner  agent.Runner
	http    *http.Server
	https   *http.Server
	models  atomic.Value
	cliVer  atomic.Value
	started time.Time
}

type Options struct {
	Config config.Config
	Log    *slog.Logger
	Runner agent.Runner
	Auth   auth.Authenticator
}

func New(opts Options) *Server {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.Runner == nil {
		opts.Runner = agent.NewRunner(opts.Log)
	}
	s := &Server{
		cfg:     opts.Config,
		log:     opts.Log,
		auth:    opts.Auth,
		runner:  opts.Runner,
		started: time.Now(),
	}
	s.cliVer.Store("unknown")
	s.models.Store(openai.MergeModelIDs(nil))
	handler := s.routes()
	s.http = newHTTPServer(opts.Config.HTTPAddr(), handler)
	if opts.Config.TLS {
		s.https = newHTTPServer(opts.Config.TLSAddr(), handler)
	}
	go s.refreshModels()
	return s
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func (s *Server) SetCLIVersion(v string) {
	if strings.TrimSpace(v) == "" {
		v = "unknown"
	}
	s.cliVer.Store(v)
}

func (s *Server) CLIVersion() string {
	if v, ok := s.cliVer.Load().(string); ok && v != "" {
		return v
	}
	return "unknown"
}

func (s *Server) Handler() http.Handler {
	if s.http != nil && s.http.Handler != nil {
		return s.http.Handler
	}
	if s.https != nil {
		return s.https.Handler
	}
	return http.NotFoundHandler()
}

func (s *Server) Start() error {
	httpLn, err := net.Listen("tcp", s.cfg.HTTPAddr())
	if err != nil {
		return fmt.Errorf("http listen %s: %w", s.cfg.HTTPAddr(), err)
	}

	var tlsLn net.Listener
	if s.cfg.TLS {
		certFile, keyFile, err := tlsutil.Ensure(s.cfg)
		if err != nil {
			_ = httpLn.Close()
			return err
		}
		tlsCfg, err := tlsutil.TLSConfig(certFile, keyFile)
		if err != nil {
			_ = httpLn.Close()
			return err
		}
		raw, err := net.Listen("tcp", s.cfg.TLSAddr())
		if err != nil {
			_ = httpLn.Close()
			return fmt.Errorf("https listen %s: %w", s.cfg.TLSAddr(), err)
		}
		if s.https == nil {
			s.https = newHTTPServer(s.cfg.TLSAddr(), s.Handler())
		}
		s.https.TLSConfig = tlsCfg
		tlsLn = tls.NewListener(raw, tlsCfg)
		s.log.Info("server listening", "addr", s.cfg.HTTPBaseURL(), "tls", false)
		s.log.Info("server listening", "addr", s.cfg.TLSBaseURL(), "tls", true, "cert", certFile)
	} else {
		s.log.Info("server listening", "addr", s.cfg.HTTPBaseURL(), "tls", false)
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- serve("http", s.http, httpLn)
	}()
	if tlsLn != nil {
		go func() {
			errCh <- serve("https", s.https, tlsLn)
		}()
	}

	err = <-errCh
	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.Shutdown(shutCtx)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func serve(name string, srv *http.Server, ln net.Listener) error {
	err := srv.Serve(ln)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("%s serve: %w", name, err)
}

func (s *Server) Shutdown(ctx context.Context) error {
	var httpErr, httpsErr error
	if s.http != nil {
		httpErr = s.http.Shutdown(ctx)
	}
	if s.https != nil {
		httpsErr = s.https.Shutdown(ctx)
	}
	if httpErr != nil {
		return httpErr
	}
	return httpsErr
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/responses", s.handleResponses)
	mux.HandleFunc("/", s.handleNotFound)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := newRequestID()
		w.Header().Set("X-Request-Id", reqID)
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID, reqID))

		s.setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.URL.Path != "/health" {
			if err := s.auth.AuthorizeRequest(r); err != nil {
				s.log.Warn("unauthorized", "id", reqID, "path", r.URL.Path, "err", err.Error())
				writeJSON(w, http.StatusUnauthorized, openai.NewError(err.Error(), "invalid_request_error", "unauthorized"))
				return
			}
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic", "id", reqID, "err", recovered)
				if !rec.wrote {
					writeJSON(rec, http.StatusInternalServerError, openai.NewError("internal server error", "server_error", nil))
				}
			}
			s.log.Info("request",
				"id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()
		next.ServeHTTP(rec, r)
	})
}

func (s *Server) setCORS(w http.ResponseWriter) {
	origin := s.cfg.CORSAllowOrigin
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, openai.NewError("method not allowed", "invalid_request_error", "method_not_allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"provider":    config.AppName,
		"cli_version": s.CLIVersion(),
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, openai.NewError("method not allowed", "invalid_request_error", "method_not_allowed"))
		return
	}
	ids, _ := s.models.Load().([]string)
	if len(ids) == 0 {
		s.refreshModels()
		ids, _ = s.models.Load().([]string)
	}
	writeJSON(w, http.StatusOK, openai.CreateModelListFrom(ids))
}

func (s *Server) refreshModels() {
	bin := s.cfg.AgentBin
	if bin == "" {
		bin = "agent"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--list-models")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	ids := openai.ParseListModels(string(out))
	if len(ids) == 0 {
		return
	}
	s.models.Store(openai.MergeModelIDs(ids))
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, openai.NewError("Not found", "invalid_request_error", "not_found"))
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, openai.NewError("method not allowed", "invalid_request_error", "method_not_allowed"))
		return
	}
	reqID := requestIDFrom(r.Context())
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, openai.NewError("request body too large", "invalid_request_error", "body_too_large"))
		return
	}
	var req openai.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, openai.NewError("invalid JSON body", "invalid_request_error", "invalid_json"))
		return
	}
	if len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, openai.NewError("messages is required and must be a non-empty array", "invalid_request_error", "invalid_messages"))
		return
	}

	cli := openai.OpenAIToCLI(req)
	s.log.Info("chat", "id", reqID, "model", req.Model, "cli_model", cli.Model, "stream", req.Stream)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.cfg.RequestTimeoutMS)*time.Millisecond)
	defer cancel()

	events := s.runner.Run(ctx, cli.Prompt, agent.Options{
		Model:   cli.Model,
		APIKey:  s.cfg.CursorAPIKey,
		Bin:     s.cfg.AgentBin,
		Timeout: time.Duration(s.cfg.RequestTimeoutMS) * time.Millisecond,
	})

	if req.Stream {
		s.writeStream(w, r, reqID, cli.Model, events)
		return
	}
	s.writeJSONCompletion(w, reqID, cli.Model, events)
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, openai.NewError("method not allowed", "invalid_request_error", "method_not_allowed"))
		return
	}
	reqID := requestIDFrom(r.Context())
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, openai.NewError("request body too large", "invalid_request_error", "body_too_large"))
		return
	}
	var req openai.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, openai.NewError("invalid JSON body", "invalid_request_error", "invalid_json"))
		return
	}
	chat, err := req.ToChatRequest()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, openai.NewError(err.Error(), "invalid_request_error", "invalid_input"))
		return
	}
	cli := openai.OpenAIToCLI(chat)
	s.log.Info("responses", "id", reqID, "model", req.Model, "cli_model", cli.Model, "stream", req.Stream)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.cfg.RequestTimeoutMS)*time.Millisecond)
	defer cancel()
	events := s.runner.Run(ctx, cli.Prompt, agent.Options{
		Model:   cli.Model,
		APIKey:  s.cfg.CursorAPIKey,
		Bin:     s.cfg.AgentBin,
		Timeout: time.Duration(s.cfg.RequestTimeoutMS) * time.Millisecond,
	})
	if req.Stream {
		s.writeResponsesStream(w, r, reqID, cli.Model, events)
		return
	}
	s.writeJSONResponse(w, reqID, cli.Model, events)
}

func (s *Server) writeStream(w http.ResponseWriter, r *http.Request, reqID, model string, events <-chan agent.Event) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, openai.NewError("streaming not supported", "server_error", nil))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ":ok\n\n")
	flusher.Flush()

	isFirst := true
	lastModel := model
	complete := false

	writeSSE := func(v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = io.WriteString(w, "data: "+string(raw)+"\n\n")
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				if !complete {
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
					flusher.Flush()
				}
				return
			}
			switch ev.Type {
			case agent.EventDelta:
				if ev.Text == "" {
					continue
				}
				if ev.Model != "" {
					lastModel = ev.Model
				}
				writeSSE(openai.CreateStreamChunk(reqID, lastModel, ev.Text, isFirst))
				isFirst = false
			case agent.EventResult:
				complete = true
				if ev.Model != "" {
					lastModel = ev.Model
				}
				writeSSE(openai.CreateDoneChunk(reqID, lastModel))
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
			case agent.EventError:
				s.log.Error("stream error", "id", reqID, "err", ev.Err)
				msg := "agent error"
				if ev.Err != nil {
					msg = ev.Err.Error()
				}
				writeSSE(openai.NewError(msg, "server_error", nil))
			}
		}
	}
}

func (s *Server) writeJSONCompletion(w http.ResponseWriter, reqID, model string, events <-chan agent.Event) {
	var result *agent.Event
	for ev := range events {
		switch ev.Type {
		case agent.EventResult:
			copyEv := ev
			result = &copyEv
		case agent.EventError:
			s.log.Error("chat error", "id", reqID, "err", ev.Err)
			msg := "agent error"
			if ev.Err != nil {
				msg = ev.Err.Error()
			}
			writeJSON(w, http.StatusInternalServerError, openai.NewError(msg, "server_error", nil))
			return
		}
	}
	if result == nil {
		writeJSON(w, http.StatusInternalServerError, openai.NewError("CLI exited without producing a result", "server_error", nil))
		return
	}
	useModel := model
	if result.Model != "" {
		useModel = result.Model
	}
	writeJSON(w, http.StatusOK, openai.CreateChatResponse(reqID, useModel, result.Text))
}

func (s *Server) writeJSONResponse(w http.ResponseWriter, reqID, model string, events <-chan agent.Event) {
	var result *agent.Event
	for ev := range events {
		switch ev.Type {
		case agent.EventResult:
			copyEv := ev
			result = &copyEv
		case agent.EventError:
			s.log.Error("responses error", "id", reqID, "err", ev.Err)
			msg := "agent error"
			if ev.Err != nil {
				msg = ev.Err.Error()
			}
			writeJSON(w, http.StatusInternalServerError, openai.NewError(msg, "server_error", nil))
			return
		}
	}
	if result == nil {
		writeJSON(w, http.StatusInternalServerError, openai.NewError("CLI exited without producing a result", "server_error", nil))
		return
	}
	useModel := model
	if result.Model != "" {
		useModel = result.Model
	}
	writeJSON(w, http.StatusOK, openai.CreateResponsesResult(reqID, useModel, result.Text))
}

func (s *Server) writeResponsesStream(w http.ResponseWriter, r *http.Request, reqID, model string, events <-chan agent.Event) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, openai.NewError("streaming not supported", "server_error", nil))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeEvent := func(ev openai.ResponseStreamEvent) {
		raw, err := json.Marshal(ev)
		if err != nil {
			return
		}
		_, _ = io.WriteString(w, "event: "+ev.Type+"\n")
		_, _ = io.WriteString(w, "data: "+string(raw)+"\n\n")
		flusher.Flush()
	}

	lastModel := model
	var full strings.Builder
	writeEvent(openai.ResponseStreamEvent{Type: "response.created"})
	writeEvent(openai.ResponseStreamEvent{
		Type: "response.output_item.added",
		Item: openai.ResponseItem{ID: "msg_" + reqID, Type: "message", Role: "assistant", Status: "in_progress"},
	})

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				res := openai.CreateResponsesResult(reqID, lastModel, full.String())
				writeEvent(openai.ResponsesCompleted(res))
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			switch ev.Type {
			case agent.EventDelta:
				if ev.Text == "" {
					continue
				}
				if ev.Model != "" {
					lastModel = ev.Model
				}
				full.WriteString(ev.Text)
				writeEvent(openai.ResponsesTextDelta(ev.Text))
			case agent.EventResult:
				if ev.Model != "" {
					lastModel = ev.Model
				}
				if ev.Text != "" && full.Len() == 0 {
					full.WriteString(ev.Text)
				}
				res := openai.CreateResponsesResult(reqID, lastModel, full.String())
				writeEvent(openai.ResponsesCompleted(res))
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			case agent.EventError:
				s.log.Error("responses stream error", "id", reqID, "err", ev.Err)
				msg := "agent error"
				if ev.Err != nil {
					msg = ev.Err.Error()
				}
				writeSSE := func(v any) {
					raw, err := json.Marshal(v)
					if err != nil {
						return
					}
					_, _ = io.WriteString(w, "data: "+string(raw)+"\n\n")
					flusher.Flush()
				}
				writeSSE(openai.NewError(msg, "server_error", nil))
				return
			}
		}
	}
}

type ctxKey int

const ctxKeyRequestID ctxKey = 1

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return newRequestID()
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
	mu     sync.Mutex
}

func (s *statusRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wrote {
		return
	}
	s.wrote = true
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.mu.Lock()
	if !s.wrote {
		s.wrote = true
		s.status = http.StatusOK
	}
	s.mu.Unlock()
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
