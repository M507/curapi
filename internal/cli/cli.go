// Package cli is the curapi command-line entrypoint.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/agent"
	"github.com/tageecc/cursor-agent-api-proxy/internal/auth"
	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
	"github.com/tageecc/cursor-agent-api-proxy/internal/daemon"
	"github.com/tageecc/cursor-agent-api-proxy/internal/logger"
	"github.com/tageecc/cursor-agent-api-proxy/internal/server"
	"github.com/tageecc/cursor-agent-api-proxy/internal/service"
)

var (
	Version           = "2.0.0"
	Stdout  io.Writer = os.Stdout
	Stderr  io.Writer = os.Stderr
)

type options struct {
	configPath  string
	command     string
	port        int
	help        bool
	version     bool
	daemonChild bool
}

func Execute(args []string) int {
	opts, err := parseArgs(args[1:])
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	if opts.help || opts.command == "help" || opts.command == "-h" || opts.command == "--help" {
		fmt.Fprint(Stdout, usage())
		return 0
	}
	if opts.version || opts.command == "version" {
		fmt.Fprintf(Stdout, "%s %s\n", config.AppName, Version)
		return 0
	}

	switch opts.command {
	case "stop":
		return cmdStop(opts)
	case "status":
		return cmdStatus(opts)
	case "restart":
		return cmdRestart(opts)
	case "install":
		return cmdInstall(opts, false)
	case "reinstall":
		return cmdInstall(opts, true)
	case "uninstall":
		return cmdUninstall()
	case "run":
		return cmdRun(opts)
	case "start", "":
		return cmdStart(opts)
	default:
		if opts.command != "" && isPort(opts.command) {
			port, _ := strconv.Atoi(opts.command)
			opts.port = port
			opts.command = "start"
			return cmdStart(opts)
		}
		fmt.Fprintf(Stderr, "Unknown command: %s\n\n", opts.command)
		fmt.Fprint(Stderr, usage())
		return 1
	}
}

func parseArgs(args []string) (options, error) {
	var opts options
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			opts.help = true
		case a == "--version" || a == "-v":
			opts.version = true
		case a == "--daemon-child":
			opts.daemonChild = true
		case a == "--config" || a == "-c":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--config requires a path")
			}
			i++
			opts.configPath = args[i]
		case strings.HasPrefix(a, "--config="):
			opts.configPath = strings.TrimPrefix(a, "--config=")
		case a == "--port" || a == "-p":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--port requires a number")
			}
			i++
			port, err := parsePort(args[i])
			if err != nil {
				return opts, err
			}
			opts.port = port
		case strings.HasPrefix(a, "--port="):
			port, err := parsePort(strings.TrimPrefix(a, "--port="))
			if err != nil {
				return opts, err
			}
			opts.port = port
		case strings.HasPrefix(a, "-"):
			return opts, fmt.Errorf("unknown flag: %s", a)
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) > 0 {
		opts.command = positional[0]
	}
	if len(positional) > 1 {
		if isPort(positional[1]) {
			port, err := parsePort(positional[1])
			if err != nil {
				return opts, err
			}
			opts.port = port
		} else {
			return opts, fmt.Errorf("unexpected argument: %s", positional[1])
		}
	}
	if len(positional) > 2 {
		return opts, fmt.Errorf("unexpected argument: %s", positional[2])
	}
	return opts, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port: %s", s)
	}
	return port, nil
}

func isPort(s string) bool {
	_, err := parsePort(s)
	return err == nil
}

func usage() string {
	return fmt.Sprintf(`Usage: %s [command] [options]

OpenAI-compatible API proxy for the Cursor CLI.

Commands:
  start [port]    Start in background (default if no command given)
  stop            Stop the background server
  restart [port]  Restart the background server
  status          Check if the server is running
  run [port]      Run in the foreground (for debugging)
  install         Install as a system service (refreshes if already installed)
  reinstall       Stop, replace the binary/unit, and start the service again
  uninstall       Remove the system service
  version         Print version

Options:
  --config, -c    Path to env.json (default: ./env.json, then ~/.%s/env.json)
  --port, -p      Listen port (overrides env.json)
  -h, --help      Show this help

Logs: ~/.%s/server.log
Config: ~/.%s/env.json
`, config.AppName, config.AppName, config.AppName, config.AppName)
}

func loadConfig(opts options) (config.Config, error) {
	cfg, created, err := config.LoadOrCreate(opts.configPath)
	if err != nil {
		return config.Config{}, err
	}
	if created {
		fmt.Fprintf(Stdout, "Created %s (mode 0600)\n", cfg.Path)
		fmt.Fprintf(Stdout, "Authorization token (save this; it is also in env.json):\n  %s\n", cfg.FirstAuthzToken())
		fmt.Fprintf(Stdout, "Clients must send: Authorization: Bearer <token>\n\n")
	}
	if opts.port > 0 {
		cfg.Port = opts.port
	}
	if err := cfg.EnsureStateDir(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func cmdStatus(opts options) int {
	cfg, err := loadConfig(opts)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	st := daemon.New(cfg).Status()
	if st.Running {
		fmt.Fprintf(Stdout, "%s is running (pid: %d).\n  Logs: %s\n", config.AppName, st.PID, st.LogFile)
		return 0
	}
	fmt.Fprintf(Stdout, "%s is not running.\n", config.AppName)
	fmt.Fprintf(Stdout, "  Run `%s start` to start.\n", config.AppName)
	return 0
}

func cmdStop(opts options) int {
	cfg, err := loadConfig(opts)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	pid, err := daemon.New(cfg).Stop()
	if err == daemon.ErrNotRunning {
		fmt.Fprintf(Stdout, "%s is not running.\n", config.AppName)
		return 0
	}
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	fmt.Fprintf(Stdout, "%s stopped (was pid: %d).\n", config.AppName, pid)
	return 0
}

func cmdStart(opts options) int {
	cfg, err := loadConfig(opts)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	m := daemon.New(cfg)
	if pid, ok := m.RunningPID(); ok {
		fmt.Fprintf(Stdout, "%s is already running (pid: %d).\n", config.AppName, pid)
		fmt.Fprintf(Stdout, "Run `%s restart` to restart.\n", config.AppName)
		return 0
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(Stderr, "locate executable: %v\n", err)
		return 1
	}
	args := []string{"run", "--config", cfg.Path, "--port", strconv.Itoa(cfg.Port), "--daemon-child"}
	pid, err := m.Start(exe, args, os.Environ())
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	base := cfg.HTTPBaseURL()
	https := ""
	if cfg.TLS {
		https = cfg.TLSBaseURL()
	}
	fmt.Fprintf(Stdout, `
  %s running (pid %d)
  HTTP  : %s/v1
  HTTPS : %s/v1
  Health: %s/health
  Logs  : %s
`, config.AppName, pid, base, orDash(https), base, cfg.LogPath())
	if https != "" {
		fmt.Fprintf(Stdout, "  HTTPS health: %s/health\n", https)
	}
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "(disabled)"
	}
	return s
}

func cmdRestart(opts options) int {
	_ = cmdStop(opts)
	return cmdStart(opts)
}

func cmdInstall(opts options, reinstall bool) int {
	cfg, err := loadConfig(opts)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	_, _ = daemon.New(cfg).Stop()
	inst := service.NewInstaller()
	if opts.configPath != "" {
		inst.Paths.EnvFile = cfg.Path
	}
	var runErr error
	if reinstall {
		runErr = inst.Reinstall()
	} else {
		fmt.Fprintf(Stdout, "Installing %s as auto-start service (existing install will be refreshed)...\n\n", service.Name)
		runErr = inst.Install()
	}
	if runErr != nil {
		fmt.Fprintln(Stderr, runErr)
		return 1
	}
	return 0
}

func cmdUninstall() int {
	inst := service.NewInstaller()
	if err := inst.Uninstall(); err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	return 0
}

func cmdRun(opts options) int {
	cfg, err := loadConfig(opts)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		return 1
	}
	logRes, err := logger.Setup(cfg, nil)
	if err != nil {
		fmt.Fprintf(Stderr, "setup logger: %v\n", err)
		return 1
	}
	defer logRes.Closer()
	log := logRes.Logger

	cliVersion := ""
	if !cfg.SkipCLICheck {
		fmt.Fprintln(Stdout, "Checking Cursor CLI (agent)...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ver, err := agent.Verify(ctx, cfg.AgentBin, nil)
		cancel()
		if err != nil {
			fmt.Fprintf(Stderr, "  %v\n\nPlease install and authenticate the Cursor CLI first:\n", err)
			if isWindows() {
				fmt.Fprintln(Stderr, "  irm 'https://cursor.com/install?win32=true' | iex")
				fmt.Fprintln(Stderr, "  agent login")
			} else {
				fmt.Fprintln(Stderr, "  curl https://cursor.com/install -fsS | bash")
				fmt.Fprintln(Stderr, "  agent login")
			}
			return 1
		}
		fmt.Fprintf(Stdout, "  Cursor CLI: %s\n", ver)
		log.Info("cursor cli ok", "version", ver)
		cliVersion = ver
	}

	if cfg.AuthRequired {
		log.Info("authz enabled", "tokens", len(cfg.AuthzTokens), "config", cfg.Path)
	} else {
		log.Warn("authz disabled; API is reachable without a bearer token")
	}

	srv := server.New(server.Options{
		Config: cfg,
		Log:    log,
		Auth:   auth.New(cfg.AuthRequired, cfg.AuthzTokens),
	})
	if cliVersion != "" {
		srv.SetCLIVersion(cliVersion)
	}

	mgr := daemon.New(cfg)
	if err := mgr.RegisterForeground(); err != nil {
		log.Warn("could not write pid file", "err", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	fmt.Fprintf(Stdout, "\n  HTTP     : %s/v1\n  Health   : %s/health\n", cfg.HTTPBaseURL(), cfg.HTTPBaseURL())
	if cfg.TLS {
		fmt.Fprintf(Stdout, "  HTTPS    : %s/v1\n  Health   : %s/health\n  Cert     : %s\n", cfg.TLSBaseURL(), cfg.TLSBaseURL(), cfg.TLSCertFile)
	}
	fmt.Fprintf(Stdout, "  Config   : %s\n  Logs     : %s\n", cfg.Path, cfg.LogPath())
	if cfg.AuthRequired {
		fmt.Fprintln(Stdout, "  Auth     : Authorization: Bearer <token from env.json>")
	}
	fmt.Fprint(Stdout, "\n  Press Ctrl+C to stop.\n\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil {
			fmt.Fprintf(Stderr, "Failed to start server: %v\n", err)
			mgr.ClearPID()
			return 1
		}
		return 0
	case <-ctx.Done():
		fmt.Fprintln(Stdout, "\nShutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		mgr.ClearPID()
		return 0
	}
}

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}
