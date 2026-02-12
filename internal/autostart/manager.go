package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"anthropic-gateway/internal/config"
)

const DefaultLabel = "com.anthropic-gateway.proxy"

type Status struct {
	Label     string
	PlistPath string
	Installed bool
	Loaded    bool
}

type Manager struct {
	label string
}

func NewManager(label string) *Manager {
	if strings.TrimSpace(label) == "" {
		label = DefaultLabel
	}
	return &Manager{label: label}
}

func (m *Manager) Label() string {
	return m.label
}

func (m *Manager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", m.label+".plist"), nil
}

func (m *Manager) Install(configPath string) error {
	if runtime.GOOS != "darwin" {
		return unsupportedErr()
	}
	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("missing required config path")
	}

	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if _, err := os.Stat(absConfig); err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}
	if _, err := config.Load(absConfig); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	if realPath, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = realPath
	}

	plistPath, err := m.plistPath()
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("create launch agent directory: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	logDir := filepath.Join(home, "Library", "Logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	stdoutLog := filepath.Join(logDir, m.label+".out.log")
	stderrLog := filepath.Join(logDir, m.label+".err.log")
	plistContent := renderLaunchAgentPlist(m.label, executablePath, absConfig, stdoutLog, stderrLog)
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = runLaunchctl("bootout", domain, plistPath)
	if err := runLaunchctl("bootstrap", domain, plistPath); err != nil {
		return err
	}
	_ = runLaunchctl("enable", domain+"/"+m.label)
	if err := runLaunchctl("kickstart", "-k", domain+"/"+m.label); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Uninstall() error {
	if runtime.GOOS != "darwin" {
		return unsupportedErr()
	}

	plistPath, err := m.plistPath()
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}

	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = runLaunchctl("bootout", domain, plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}
	return nil
}

func (m *Manager) Status() (Status, error) {
	if runtime.GOOS != "darwin" {
		return Status{}, unsupportedErr()
	}

	plistPath, err := m.plistPath()
	if err != nil {
		return Status{}, fmt.Errorf("resolve plist path: %w", err)
	}

	installed := true
	if _, err := os.Stat(plistPath); err != nil {
		if os.IsNotExist(err) {
			installed = false
		} else {
			return Status{}, fmt.Errorf("stat plist: %w", err)
		}
	}

	loaded := false
	if installed {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), m.label)
		if err := runLaunchctl("print", target); err == nil {
			loaded = true
		}
	}

	return Status{
		Label:     m.label,
		PlistPath: plistPath,
		Installed: installed,
		Loaded:    loaded,
	}, nil
}

func runLaunchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("launchctl %s failed: %s", strings.Join(args, " "), msg)
		}
		return fmt.Errorf("launchctl %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}

func unsupportedErr() error {
	return fmt.Errorf("autostart is only supported on macOS")
}
