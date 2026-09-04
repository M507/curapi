package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func (i *Installer) installPlatform() error {
	switch i.GOOS {
	case "linux":
		return i.installLinux()
	case "darwin":
		return i.installDarwin()
	case "windows":
		return i.installWindows()
	default:
		return fmt.Errorf("unsupported platform: %s", i.GOOS)
	}
}

func (i *Installer) uninstallPlatform() error {
	switch i.GOOS {
	case "linux":
		return i.uninstallLinux()
	case "darwin":
		return i.uninstallDarwin()
	case "windows":
		return i.uninstallWindows()
	default:
		return fmt.Errorf("unsupported platform: %s", i.GOOS)
	}
}

func (i *Installer) uninstallQuiet() error {
	_ = i.uninstallPlatform()
	return nil
}

func (i *Installer) installLinux() error {
	unit := SystemdUnit(i.Paths.Binary, i.Paths.EnvFile, i.agentPATH())
	if err := writeFile(i.Paths.SystemdUserUnit, unit, 0o644); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}
	if _, err := i.Exec("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if _, err := i.Exec("systemctl", "--user", "enable", "--now", Name); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	i.printf("Service installed and started.\n")
	i.printf("  Binary: %s\n", i.Paths.Binary)
	i.printf("  Config: %s\n", i.Paths.EnvFile)
	i.printf("  Unit:   %s\n", i.Paths.SystemdUserUnit)
	i.printf("  Status: systemctl --user status %s\n", Name)
	i.printf("  Logs:   journalctl --user -u %s -f\n", Name)
	i.printf("          also %s\n", i.Paths.LogFile)
	return nil
}

func (i *Installer) uninstallLinux() error {
	i.disableSystemd(Name, i.Paths.SystemdUserUnit)
	if LegacyName != Name && i.Paths.SystemdUserDir != "" {
		i.disableSystemd(LegacyName, filepath.Join(i.Paths.SystemdUserDir, LegacyName+".service"))
	}
	_, _ = i.Exec("systemctl", "--user", "daemon-reload")
	i.printf("Service uninstalled.\n")
	return nil
}

func (i *Installer) disableSystemd(name, unit string) {
	if name != "" {
		_, _ = i.Exec("systemctl", "--user", "disable", "--now", name)
	}
	if unit != "" {
		_ = os.Remove(unit)
	}
}

func (i *Installer) installDarwin() error {
	plist := LaunchdPlist(DarwinLabel, i.Paths.Binary, i.Paths.EnvFile, i.agentPATH(), i.Paths.LogFile)
	if err := writeFile(i.Paths.LaunchPlist, plist, 0o644); err != nil {
		return fmt.Errorf("write LaunchAgent: %w", err)
	}
	uid := strconv.Itoa(os.Getuid())
	_, _ = i.Exec("launchctl", "bootout", "gui/"+uid, i.Paths.LaunchPlist)
	if _, err := i.Exec("launchctl", "bootstrap", "gui/"+uid, i.Paths.LaunchPlist); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	i.printf("Service installed and started.\n")
	i.printf("  Binary: %s\n", i.Paths.Binary)
	i.printf("  Config: %s\n", i.Paths.EnvFile)
	i.printf("  Plist:  %s\n", i.Paths.LaunchPlist)
	i.printf("  Logs:   %s\n", i.Paths.LogFile)
	return nil
}

func (i *Installer) uninstallDarwin() error {
	uid := strconv.Itoa(os.Getuid())
	_, _ = i.Exec("launchctl", "bootout", "gui/"+uid, i.Paths.LaunchPlist)
	if err := os.Remove(i.Paths.LaunchPlist); err != nil && !os.IsNotExist(err) {
		return err
	}
	if i.Home != "" && LegacyDarwinLabel != DarwinLabel {
		legacy := filepath.Join(i.Home, "Library", "LaunchAgents", LegacyDarwinLabel+".plist")
		_, _ = i.Exec("launchctl", "bootout", "gui/"+uid, legacy)
		_ = os.Remove(legacy)
	}
	i.printf("Service uninstalled.\n")
	return nil
}

func (i *Installer) installWindows() error {
	_, _ = i.Exec("schtasks", "/End", "/TN", WindowsTask)
	_, _ = i.Exec("schtasks", "/Delete", "/TN", WindowsTask, "/F")
	tr := WindowsTaskCommand(i.Paths.Binary, i.Paths.EnvFile)
	if _, err := i.Exec("schtasks", "/Create", "/TN", WindowsTask, "/TR", tr, "/SC", "ONLOGON", "/RL", "LIMITED", "/F"); err != nil {
		return fmt.Errorf("schtasks create: %w", err)
	}
	if _, err := i.Exec("schtasks", "/Run", "/TN", WindowsTask); err != nil {
		return fmt.Errorf("schtasks run: %w", err)
	}
	i.printf("Service installed and started.\n")
	i.printf("  Task:   %s\n", WindowsTask)
	i.printf("  Binary: %s\n", i.Paths.Binary)
	i.printf("  Config: %s\n", i.Paths.EnvFile)
	return nil
}

func (i *Installer) uninstallWindows() error {
	_, _ = i.Exec("schtasks", "/End", "/TN", WindowsTask)
	_, _ = i.Exec("schtasks", "/Delete", "/TN", WindowsTask, "/F")
	if LegacyWindowsTask != WindowsTask {
		_, _ = i.Exec("schtasks", "/End", "/TN", LegacyWindowsTask)
		_, _ = i.Exec("schtasks", "/Delete", "/TN", LegacyWindowsTask, "/F")
	}
	i.printf("Service uninstalled.\n")
	return nil
}
