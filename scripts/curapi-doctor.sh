#!/usr/bin/env bash
# curapi-doctor: detect common curapi failures and repair them.
set -u

NAME="curapi"
LEGACY_NAME="cursor-agent-api"
BIN_DIR="${HOME}/.local/bin"
BIN="${BIN_DIR}/${NAME}"
STATE_DIR="${HOME}/.${NAME}"
ENV_FILE="${STATE_DIR}/env.json"
LOG_FILE="${STATE_DIR}/server.log"
PID_FILE="${STATE_DIR}/pid"
UNIT_DIR="${HOME}/.config/systemd/user"
UNIT_FILE="${UNIT_DIR}/${NAME}.service"
LEGACY_UNIT="${UNIT_DIR}/${LEGACY_NAME}.service"
HTTP_HEALTH="http://127.0.0.1:4646/health"
HTTPS_HEALTH="https://127.0.0.1:4647/health"

FIX=1
ISSUES=0
FIXED=0
FAILED=0

usage() {
	cat <<'EOF'
Usage: curapi-doctor.sh [options]

Check curapi for common problems and fix them when possible
(restart the service, restore PATH, remove a leftover unit, etc.).

Options:
  -h, --help     Show this help and exit
  --check        Report issues only; do not change anything
  --fix          Apply repairs (default)

Checks:
  - ~/.local/bin on PATH and curapi binary present
  - env.json exists and is mode 0600
  - leftover cursor-agent-api service/binary
  - stale PID file
  - systemd user unit installed, enabled, and running
  - HTTP/HTTPS /health returns 200
  - ports 4646/4647 listening
  - Cursor CLI (agent) on PATH

Exit codes:
  0  healthy (or all found issues were fixed)
  1  problems remain
  2  usage / invalid flag
EOF
}

log() { printf '%s\n' "$*"; }
ok() { printf '  ok     %s\n' "$*"; }
info() { printf '  info   %s\n' "$*"; }
warn() { printf '  warn   %s\n' "$*"; ISSUES=$((ISSUES + 1)); }
fail() { printf '  fail   %s\n' "$*"; FAILED=$((FAILED + 1)); }
did_fix() { printf '  fixed  %s\n' "$*"; FIXED=$((FIXED + 1)); }

want_fix() { [ "$FIX" -eq 1 ]; }

have_systemctl() {
	command -v systemctl >/dev/null 2>&1 && [ "$(uname -s)" = Linux ]
}

json_get() {
	python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get(sys.argv[2],"") or "")' "$1" "$2" 2>/dev/null || true
}

health_ok() {
	local url="$1"
	local extra=()
	[[ "$url" == https://* ]] && extra+=(-k)
	local body
	body="$(curl -sS -m 5 "${extra[@]}" "$url" 2>/dev/null)" || return 1
	printf '%s' "$body" | grep -q '"status":"ok"' || return 1
	printf '%s' "$body" | grep -q '"provider":"curapi"' || return 1
}

wait_health() {
	local i
	for i in 1 2 3 4 5 6 7 8 9 10; do
		if health_ok "$HTTP_HEALTH"; then
			return 0
		fi
		sleep 0.5
	done
	return 1
}

port_pids() {
	local port="$1"
	ss -ltnp 2>/dev/null | awk -v p=":${port}" '$4 ~ p"$" {print}' || true
}

ensure_path() {
	export PATH="${BIN_DIR}:$PATH"
	case ":$PATH:" in
	*":${BIN_DIR}:"*) ;;
	esac
	if command -v "${NAME}" >/dev/null 2>&1; then
		ok "curapi is on PATH ($(command -v "${NAME}"))"
		return
	fi
	warn "curapi is not on PATH"
	if ! want_fix; then
		return
	fi
	local rc="${HOME}/.bashrc"
	[ -n "${ZSH_VERSION:-}" ] && rc="${HOME}/.zshrc"
	if [ ! -f "$rc" ]; then
		rc="${HOME}/.profile"
	fi
	if [ -f "$rc" ] && grep -F "${BIN_DIR}" "$rc" >/dev/null 2>&1; then
		info "${BIN_DIR} already listed in ${rc}; open a new shell"
	else
		touch "$rc"
		printf '\n# added by curapi-doctor\nexport PATH="%s:$PATH"\n' "${BIN_DIR}" >>"$rc"
		did_fix "appended ${BIN_DIR} to PATH in ${rc}"
	fi
}

ensure_binary() {
	if [ -x "$BIN" ]; then
		ok "binary ${BIN}"
		return
	fi
	warn "binary missing: ${BIN}"
	if ! want_fix; then
		return
	fi
	local here
	here="$(cd "$(dirname "$0")/.." && pwd)"
	if [ -x "${here}/bin/${NAME}" ]; then
		mkdir -p "$BIN_DIR"
		install -m 755 "${here}/bin/${NAME}" "$BIN"
		did_fix "copied ${here}/bin/${NAME} to ${BIN}"
		return
	fi
	if [ -f "${here}/Makefile" ] && command -v go >/dev/null 2>&1; then
		(cd "$here" && make install)
		if [ -x "$BIN" ]; then
			did_fix "built and installed ${BIN}"
			return
		fi
	fi
	fail "could not install ${BIN}; run make install from the repo"
}

ensure_env() {
	if [ ! -f "$ENV_FILE" ]; then
		warn "config missing: ${ENV_FILE}"
		if want_fix && [ -x "$BIN" ]; then
			"$BIN" --config "$ENV_FILE" status >/dev/null 2>&1 || true
			if [ -f "$ENV_FILE" ]; then
				did_fix "created ${ENV_FILE}"
			else
				fail "could not create ${ENV_FILE}"
				return
			fi
		else
			return
		fi
	else
		ok "config ${ENV_FILE}"
	fi
	local mode
	mode="$(stat -c '%a' "$ENV_FILE" 2>/dev/null || stat -f '%OLp' "$ENV_FILE" 2>/dev/null || echo "")"
	if [ "$mode" != "600" ]; then
		warn "env.json mode is ${mode:-unknown}, want 0600"
		if want_fix; then
			chmod 600 "$ENV_FILE"
			did_fix "chmod 600 ${ENV_FILE}"
		fi
	else
		ok "env.json mode 0600"
	fi
	local tokens
	tokens="$(json_get "$ENV_FILE" authz_tokens)"
	if [ -z "$tokens" ] || [ "$tokens" = "[]" ]; then
		fail "authz_tokens is empty; add a token in ${ENV_FILE}"
	else
		ok "authz_tokens present"
	fi
}

remove_legacy() {
	local found=0
	if have_systemctl && [ -f "$LEGACY_UNIT" ]; then
		found=1
		warn "legacy unit still installed: ${LEGACY_UNIT}"
		if want_fix; then
			systemctl --user disable --now "$LEGACY_NAME" >/dev/null 2>&1 || true
			rm -f "$LEGACY_UNIT"
			systemctl --user daemon-reload >/dev/null 2>&1 || true
			did_fix "removed ${LEGACY_NAME}.service"
		fi
	fi
	if [ -e "${BIN_DIR}/${LEGACY_NAME}" ]; then
		found=1
		warn "legacy binary still present: ${BIN_DIR}/${LEGACY_NAME}"
		if want_fix; then
			rm -f "${BIN_DIR}/${LEGACY_NAME}"
			did_fix "removed ${BIN_DIR}/${LEGACY_NAME}"
		fi
	fi
	if [ "$found" -eq 0 ]; then
		ok "no leftover ${LEGACY_NAME} service/binary"
	fi
}

clear_stale_pid() {
	if [ ! -f "$PID_FILE" ]; then
		ok "no stale pid file"
		return
	fi
	local pid
	pid="$(tr -d ' \n' <"$PID_FILE" 2>/dev/null || true)"
	if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
		ok "pid file live (${pid})"
		return
	fi
	warn "stale pid file ${PID_FILE} (${pid:-empty})"
	if want_fix; then
		rm -f "$PID_FILE"
		did_fix "removed stale pid file"
	fi
}

restart_service() {
	if have_systemctl && [ -f "$UNIT_FILE" ]; then
		systemctl --user daemon-reload >/dev/null 2>&1 || true
		if ! systemctl --user restart "$NAME"; then
			systemctl --user enable --now "$NAME" >/dev/null 2>&1 || true
		fi
		return 0
	fi
	if [ -x "$BIN" ]; then
		"$BIN" restart >/dev/null 2>&1 || "$BIN" start >/dev/null 2>&1 || true
	fi
}

ensure_service() {
	if ! have_systemctl; then
		info "not Linux systemd; skipping unit checks"
		if [ -x "$BIN" ]; then
			"$BIN" status || true
		fi
		return
	fi
	if [ ! -f "$UNIT_FILE" ]; then
		warn "systemd unit missing: ${UNIT_FILE}"
		if want_fix && [ -x "$BIN" ]; then
			"$BIN" install
			if [ -f "$UNIT_FILE" ]; then
				did_fix "installed ${NAME}.service"
			else
				fail "curapi install did not write ${UNIT_FILE}"
				return
			fi
		else
			return
		fi
	else
		ok "unit ${UNIT_FILE}"
	fi

	local active
	active="$(systemctl --user is-active "$NAME" 2>/dev/null || true)"
	if [ "$active" != "active" ]; then
		warn "service is ${active:-unknown}, want active"
		if want_fix; then
			systemctl --user enable "$NAME" >/dev/null 2>&1 || true
			restart_service
			active="$(systemctl --user is-active "$NAME" 2>/dev/null || true)"
			if [ "$active" = "active" ]; then
				did_fix "started ${NAME}.service"
			else
				fail "could not start ${NAME}.service (${active:-unknown})"
				systemctl --user status "$NAME" --no-pager -l | tail -20 || true
			fi
		fi
	else
		ok "service active"
	fi

	local enabled
	enabled="$(systemctl --user is-enabled "$NAME" 2>/dev/null || true)"
	if [ "$enabled" != "enabled" ]; then
		warn "service is not enabled (${enabled:-unknown})"
		if want_fix; then
			systemctl --user enable "$NAME" >/dev/null 2>&1 && did_fix "enabled ${NAME}.service" || fail "could not enable ${NAME}.service"
		fi
	else
		ok "service enabled"
	fi
}

ensure_ports_and_health() {
	local p
	for p in 4646 4647; do
		if port_pids "$p" | grep -q .; then
			ok "port ${p} is listening"
		else
			warn "port ${p} is not listening"
			if want_fix; then
				restart_service
				sleep 1
				if port_pids "$p" | grep -q .; then
					did_fix "port ${p} listening after restart"
				else
					fail "port ${p} still not listening"
				fi
			fi
		fi
	done

	if health_ok "$HTTP_HEALTH"; then
		ok "HTTP health ${HTTP_HEALTH}"
	else
		warn "HTTP health failed"
		if want_fix; then
			restart_service
			if wait_health; then
				did_fix "HTTP health recovered after restart"
			else
				fail "HTTP health still failing"
			fi
		fi
	fi

	if health_ok "$HTTPS_HEALTH"; then
		ok "HTTPS health ${HTTPS_HEALTH}"
	else
		warn "HTTPS health failed (self-signed cert is expected; curl -k is used)"
		if want_fix; then
			restart_service
			sleep 1
			if health_ok "$HTTPS_HEALTH"; then
				did_fix "HTTPS health recovered after restart"
			else
				fail "HTTPS health still failing"
			fi
		fi
	fi
}

ensure_agent() {
	if command -v agent >/dev/null 2>&1; then
		ok "Cursor CLI agent at $(command -v agent)"
		return
	fi
	if [ -x "${HOME}/.local/bin/agent" ]; then
		ok "Cursor CLI agent at ${HOME}/.local/bin/agent"
		return
	fi
	fail "Cursor CLI (agent) not found; install with: curl https://cursor.com/install -fsS | bash"
}

parse_args() {
	while [ $# -gt 0 ]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--check)
			FIX=0
			;;
		--fix)
			FIX=1
			;;
		*)
			printf 'Unknown option: %s\n\n' "$1" >&2
			usage >&2
			exit 2
			;;
		esac
		shift
	done
}

main() {
	parse_args "$@"
	export PATH="${BIN_DIR}:${HOME}/.cursor-agent/bin:/usr/local/bin:/opt/homebrew/bin:$PATH"

	if [ "$FIX" -eq 1 ]; then
		log "curapi doctor (fix mode)"
	else
		log "curapi doctor (check only)"
	fi

	ensure_path
	ensure_binary
	ensure_env
	remove_legacy
	clear_stale_pid
	ensure_service
	ensure_ports_and_health
	ensure_agent

	log ""
	if [ "$FAILED" -gt 0 ]; then
		log "done: ${ISSUES} issue(s), ${FIXED} fixed, ${FAILED} still failing"
		exit 1
	fi
	if [ "$ISSUES" -gt 0 ]; then
		log "done: ${ISSUES} issue(s), ${FIXED} fixed"
		exit 0
	fi
	log "done: healthy"
	exit 0
}

main "$@"
