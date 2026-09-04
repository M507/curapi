// Package config loads proxy settings from a local env.json file.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	AppName          = "curapi"
	LegacyAppName    = "cursor-agent-api"
	DefaultPort      = 4646
	DefaultTLSPort   = 4647
	DefaultHost      = "127.0.0.1"
	DefaultTimeoutMS = 300_000
	DefaultMaxBody   = 10 << 20
)

// Config is the on-disk configuration. Secrets live only in env.json
// (mode 0600) and are never written into systemd/launchd unit files.
type Config struct {
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	TLSPort          int      `json:"tls_port"`
	AuthRequired     bool     `json:"auth_required"`
	AuthzTokens      []string `json:"authz_tokens"`
	CursorAPIKey     string   `json:"cursor_api_key"`
	AgentBin         string   `json:"agent_bin"`
	LogLevel         string   `json:"log_level"`
	LogFormat        string   `json:"log_format"`
	LogFile          string   `json:"log_file"`
	RequestTimeoutMS int      `json:"request_timeout_ms"`
	MaxBodyBytes     int64    `json:"max_body_bytes"`
	CORSAllowOrigin  string   `json:"cors_allow_origin"`
	Debug            bool     `json:"debug"`
	SkipCLICheck     bool     `json:"skip_cli_check"`
	StateDir         string   `json:"state_dir,omitempty"`
	TLS              bool     `json:"tls"`
	TLSAuto          bool     `json:"tls_auto"`
	TLSCertFile      string   `json:"tls_cert_file"`
	TLSKeyFile       string   `json:"tls_key_file"`
	TLSHosts         []string `json:"tls_hosts"`

	// Path is the file this config was loaded from (not serialized).
	Path string `json:"-"`
}

func Default() Config {
	return Config{
		Host:             DefaultHost,
		Port:             DefaultPort,
		TLSPort:          DefaultTLSPort,
		AuthRequired:     true,
		AuthzTokens:      nil,
		AgentBin:         "agent",
		LogLevel:         "info",
		LogFormat:        "text",
		RequestTimeoutMS: DefaultTimeoutMS,
		MaxBodyBytes:     DefaultMaxBody,
		CORSAllowOrigin:  "*",
		TLS:              true,
		TLSAuto:          true,
	}
}

func StateDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "."+AppName)
	}
	return filepath.Join(os.TempDir(), "."+AppName)
}

func LegacyStateDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "."+LegacyAppName)
	}
	return filepath.Join(os.TempDir(), "."+LegacyAppName)
}

func DefaultEnvPath() string {
	return filepath.Join(StateDir(), "env.json")
}

func PIDFile(stateDir string) string {
	return filepath.Join(stateDir, "pid")
}

func DefaultLogFile(stateDir string) string {
	return filepath.Join(stateDir, "server.log")
}

// Candidates returns env.json search paths in priority order.
func Candidates() []string {
	out := make([]string, 0, 8)
	for _, key := range []string{"CURAPI_ENV_FILE", "CURSOR_AGENT_ENV_FILE"} {
		if p := strings.TrimSpace(os.Getenv(key)); p != "" {
			out = append(out, p)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(cwd, "env.json"))
	}
	out = append(out, DefaultEnvPath())
	if legacy := filepath.Join(LegacyStateDir(), "env.json"); legacy != DefaultEnvPath() {
		out = append(out, legacy)
	}
	out = append(out, "/etc/"+AppName+"/env.json", "/etc/"+LegacyAppName+"/env.json")
	return out
}

func FindEnvFile() string {
	for _, p := range Candidates() {
		st, err := os.Stat(p)
		if err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return Config{}, fmt.Errorf("config path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read env.json: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse env.json: %w", err)
	}
	cfg.Path = path
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadOrCreate loads env.json from path, or from the default search path.
// If no file exists, it writes a new one with a generated authz token.
func LoadOrCreate(explicit string) (Config, bool, error) {
	path := strings.TrimSpace(explicit)
	created := false
	if path == "" {
		path = FindEnvFile()
	}
	if path == "" {
		path = DefaultEnvPath()
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return Config{}, false, err
		}
		if _, err := Generate(path); err != nil {
			return Config{}, false, err
		}
		created = true
	}
	cfg, err := Load(path)
	if err != nil {
		return Config{}, created, err
	}
	return cfg, created, nil
}

func Generate(path string) (Config, error) {
	token, err := RandomToken(32)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	cfg.AuthRequired = true
	cfg.AuthzTokens = []string{token}
	cfg.Path = path
	if err := Write(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Write(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	cfg.applyDefaults()
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write env.json: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace env.json: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Host) == "" {
		c.Host = DefaultHost
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.TLS && c.TLSPort == 0 {
		c.TLSPort = c.Port + 1
		if c.TLSPort > 65535 {
			c.TLSPort = DefaultTLSPort
		}
	}
	if strings.TrimSpace(c.AgentBin) == "" {
		c.AgentBin = "agent"
	}
	if strings.TrimSpace(c.LogLevel) == "" {
		c.LogLevel = "info"
	}
	if strings.TrimSpace(c.LogFormat) == "" {
		c.LogFormat = "text"
	}
	if c.RequestTimeoutMS <= 0 {
		c.RequestTimeoutMS = DefaultTimeoutMS
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = DefaultMaxBody
	}
	if strings.TrimSpace(c.CORSAllowOrigin) == "" {
		c.CORSAllowOrigin = "*"
	}
	if strings.TrimSpace(c.StateDir) == "" {
		c.StateDir = StateDir()
	}
	if strings.TrimSpace(c.TLSCertFile) == "" {
		c.TLSCertFile = DefaultTLSCertFile(c.StateDir)
	}
	if strings.TrimSpace(c.TLSKeyFile) == "" {
		c.TLSKeyFile = DefaultTLSKeyFile(c.StateDir)
	}
	c.TLSHosts = cleanHosts(c.TLSHosts)
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	c.LogFormat = strings.ToLower(strings.TrimSpace(c.LogFormat))
	c.AuthzTokens = cleanTokens(c.AuthzTokens)
}

func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.TLS {
		if c.TLSPort < 1 || c.TLSPort > 65535 {
			return fmt.Errorf("invalid tls_port %d", c.TLSPort)
		}
		if c.TLSPort == c.Port {
			return fmt.Errorf("tls_port (%d) must differ from port (%d)", c.TLSPort, c.Port)
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("invalid log_level %q (use debug, info, warn, error)", c.LogLevel)
	}
	switch c.LogFormat {
	case "text", "json":
	default:
		return fmt.Errorf("invalid log_format %q (use text or json)", c.LogFormat)
	}
	if c.AuthRequired && len(c.AuthzTokens) == 0 {
		return fmt.Errorf("auth_required is true but authz_tokens is empty in %s", c.Path)
	}
	if c.TLS && !c.TLSAuto {
		if strings.TrimSpace(c.TLSCertFile) == "" || strings.TrimSpace(c.TLSKeyFile) == "" {
			return fmt.Errorf("tls is enabled with tls_auto=false but tls_cert_file/tls_key_file are empty")
		}
	}
	return nil
}

func (c Config) Addr() string {
	return c.HTTPAddr()
}

func (c Config) HTTPAddr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c Config) TLSAddr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.TLSPort))
}

func (c Config) Scheme() string {
	if c.TLS {
		return "https"
	}
	return "http"
}

func (c Config) PublicHost() string {
	switch c.Host {
	case "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return c.Host
	}
}

func (c Config) url(scheme string, port int) string {
	return scheme + "://" + net.JoinHostPort(c.PublicHost(), strconv.Itoa(port))
}

func (c Config) HTTPBaseURL() string {
	return c.url("http", c.Port)
}

func (c Config) TLSBaseURL() string {
	return c.url("https", c.TLSPort)
}

func (c Config) BaseURL() string {
	if c.TLS {
		return c.TLSBaseURL()
	}
	return c.HTTPBaseURL()
}

func DefaultTLSCertFile(stateDir string) string {
	return filepath.Join(stateDir, "tls", "cert.pem")
}

func DefaultTLSKeyFile(stateDir string) string {
	return filepath.Join(stateDir, "tls", "key.pem")
}

func (c Config) LogPath() string {
	if strings.TrimSpace(c.LogFile) != "" {
		return c.LogFile
	}
	return DefaultLogFile(c.StateDir)
}

func (c Config) FirstAuthzToken() string {
	if len(c.AuthzTokens) == 0 {
		return ""
	}
	return c.AuthzTokens[0]
}

func RandomToken(nBytes int) (string, error) {
	if nBytes < 16 {
		nBytes = 16
	}
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func cleanTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func cleanHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	seen := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" || h == "0.0.0.0" || h == "::" || h == "[::]" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func (c Config) EnsureStateDir() error {
	return os.MkdirAll(c.StateDir, 0o700)
}
