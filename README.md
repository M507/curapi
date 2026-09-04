# curapi

OpenAI-compatible API proxy for the Cursor CLI, written in Go. Any OpenAI client like Open WebUI can use a Cursor subscription.

## Prerequisites

- Go 1.23+ (to build from source)
- Active [Cursor](https://cursor.com) subscription (Pro / Business)
- Cursor CLI (`agent`) installed and authenticated

## Install the Cursor CLI

```bash
# macOS / Linux
curl https://cursor.com/install -fsS | bash

# Windows PowerShell
irm 'https://cursor.com/install?win32=true' | iex
```

```bash
agent login          # opens browser, sign in with your Cursor account
agent --list-models  # verify it works
```

Headless? Skip `agent login`, generate a key at [cursor.com/settings](https://cursor.com/settings), and put it in `env.json` as `cursor_api_key`.

## Build and run

```bash
git clone https://github.com/tageecc/cursor-agent-api-proxy.git
cd cursor-agent-api-proxy
make build
make test
./bin/curapi run
```

Install the binary onto your PATH:

```bash
make install                 # copies to ~/.local/bin/curapi
curapi install               # register OS service (refreshes if already installed)
```

On first start, if no `env.json` exists, one is created at `~/.curapi/env.json` (mode `0600`) with a generated auth token. That token is printed once. An existing `~/.cursor-agent-api/` directory is migrated to `~/.curapi/` on install.

## Configuration (`env.json`)

All settings, including authorization tokens, live in a local JSON file. Search order:

1. `--config /path/to/env.json`
2. `CURAPI_ENV_FILE` (legacy: `CURSOR_AGENT_ENV_FILE`)
3. `./env.json` (current working directory)
4. `~/.curapi/env.json`
5. `~/.cursor-agent-api/env.json` (legacy)
6. `/etc/curapi/env.json`
7. `/etc/cursor-agent-api/env.json` (legacy)

If none exist, `~/.curapi/env.json` is created automatically.

See [`env.json.example`](./env.json.example):

```json
{
  "host": "127.0.0.1",
  "port": 4646,
  "tls_port": 4647,
  "auth_required": true,
  "authz_tokens": ["replace-with-a-long-random-token"],
  "cursor_api_key": "",
  "agent_bin": "agent",
  "log_level": "info",
  "log_format": "text",
  "log_file": "",
  "request_timeout_ms": 300000,
  "max_body_bytes": 10485760,
  "cors_allow_origin": "*",
  "debug": false,
  "skip_cli_check": false,
  "tls": true,
  "tls_auto": true,
  "tls_cert_file": "",
  "tls_key_file": "",
  "tls_hosts": ["localhost", "127.0.0.1"]
}
```

| Field | Description |
|-------|-------------|
| `host` | Bind address. Default `127.0.0.1` (localhost only). |
| `port` | HTTP listen port. Default `4646`. |
| `tls_port` | HTTPS listen port. Default `4647`. Must differ from `port`. |
| `auth_required` | When `true`, `/v1/*` requires a bearer token. `/health` stays open for probes. |
| `authz_tokens` | Accepted `Authorization: Bearer` values. Compared with SHA-256 + constant time. |
| `cursor_api_key` | Optional Cursor CLI key (`CURSOR_API_KEY` for `agent`). Empty uses `agent login`. |
| `agent_bin` | Cursor CLI binary name or path. Default `agent`. |
| `log_level` | `debug`, `info`, `warn`, `error`. |
| `log_format` | `text` or `json`. |
| `log_file` | Override log path. Default `~/.curapi/server.log`. |
| `request_timeout_ms` | Per-request timeout for the CLI subprocess. |
| `debug` | Extra CLI stream logging. |
| `tls` | Serve HTTPS. Default `true`. |
| `tls_auto` | Generate a self-signed cert (ECDSA P-256, 1 year) if missing or near expiry. |
| `tls_cert_file` / `tls_key_file` | PEM paths. Defaults: `~/.curapi/tls/cert.pem` and `key.pem`. |
| `tls_hosts` | Extra DNS/IP names embedded in the auto-generated certificate. |

The file is written with mode `0600`. Tokens are never copied into systemd/launchd/Task Scheduler unit files.

### Calling the API

```bash
TOKEN=$(python3 -c 'import json; print(json.load(open("env.json"))["authz_tokens"][0])')

curl http://127.0.0.1:4646/health
curl -k https://127.0.0.1:4647/health

curl -X POST http://127.0.0.1:4646/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Hello!"}]}'
```

HTTPS uses a self-signed certificate. Use `curl -k` (or trust `~/.curapi/tls/cert.pem`). Set `tls_auto` to `false` and point `tls_cert_file` / `tls_key_file` at your own PEM files to use a real certificate.

Placeholder keys such as `not-needed` are **not** accepted unless they are explicitly listed in `authz_tokens`.

## Commands

```bash
curapi                 # start in background
curapi start [port]
curapi stop
curapi restart [port]
curapi status
curapi run [port]      # foreground (debugging)
curapi install         # OS service; reinstalls/refreshes if present
curapi reinstall       # explicit refresh: stop, replace binary/unit, start
curapi uninstall       # remove OS service
curapi --help
```

Flags: `--config path/to/env.json`, `--port 8080`.

Logs: `~/.curapi/server.log` (request id, method, path, status, duration). Authorization values are redacted.

## System service

```bash
curapi install      # first time, or again to refresh the existing install
curapi reinstall    # same refresh path
curapi uninstall
```

`install` and `reinstall` both:

1. Stop a PID-file daemon if one is running
2. Remove the previous OS unit if it exists (including the legacy `cursor-agent-api` unit)
3. Copy the current binary to `~/.local/bin/curapi`
4. Keep (or create) `~/.curapi/env.json`
5. Write a fresh unit file and enable/start it

Platforms:

- Linux → systemd user service (`~/.config/systemd/user/curapi.service`)
- macOS → LaunchAgent (`~/Library/LaunchAgents/com.curapi.plist`)
- Windows → Task Scheduler (`CurAPI`)

On Linux, for start-at-boot while not logged in: `loginctl enable-linger $USER`.

## Use with OpenClaw

Provider type: **Custom Provider** (OpenAI-compatible)

- HTTP: `http://127.0.0.1:4646/v1`
- HTTPS: `https://127.0.0.1:4647/v1` (self-signed; trust the generated cert or disable TLS verification in the client)
- API Key: a token from `authz_tokens` in `env.json`
- Default model: `auto`

## Use with Open WebUI

Add an OpenAI connection:

- URL: `http://127.0.0.1:4646/v1` (or `https://127.0.0.1:4647/v1` with TLS verify off for the auto-generated cert)
- API key: a token from `authz_tokens` in `env.json`
- API type: **Responses** (`POST /v1/responses`) or **Chat Completions** (`POST /v1/chat/completions`)
- Model: `composer-2.5` or `auto`. `composer-1` is accepted and mapped to `composer-2.5`.

If Open WebUI runs in Docker without host networking, `127.0.0.1` inside the container is not this proxy — use the host IP or `--network host`.

## API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | no | Health check |
| `/v1/models` | GET | yes | List models |
| `/v1/chat/completions` | POST | yes | Chat completion (`stream: true` supported) |
| `/v1/responses` | POST | yes | OpenAI Responses API (`stream: true` supported) |

## How it works

```
Client  →  POST /v1/chat/completions (OpenAI format + Bearer token)
        →  curapi
        →  spawn agent CLI (stream-json)
        →  Cursor subscription
        →  AI response → OpenAI format → Client
```

## Development

```bash
make test
make vet
make build
```

## License

MIT
