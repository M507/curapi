package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SystemdUnit(binary, envFile, pathEnv string) string {
	return fmt.Sprintf(`[Unit]
Description=curapi
After=network.target

[Service]
Type=simple
ExecStart=%s run --config %s
Restart=on-failure
RestartSec=5
Environment=PATH=%s

[Install]
WantedBy=default.target
`, shellQuote(binary), shellQuote(envFile), pathEnv)
}

func LaunchdPlist(label, binary, envFile, pathEnv, logFile string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>run</string>
    <string>--config</string>
    <string>%s</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, xmlEscape(label), xmlEscape(binary), xmlEscape(envFile), xmlEscape(pathEnv), xmlEscape(logFile), xmlEscape(logFile))
}

func WindowsTaskCommand(binary, envFile string) string {
	return fmt.Sprintf(`"%s" run --config "%s"`, binary, envFile)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

func writeFile(path, contents string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), mode)
}
