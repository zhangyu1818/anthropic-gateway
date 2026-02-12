package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"

	"anthropic-gateway/internal/autostart"
)

func runAutostart(args []string, logger *slog.Logger) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("autostart command is only supported on macOS")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: anthropic-gateway autostart <install|uninstall|status> [flags]")
	}

	manager := autostart.NewManager("")
	switch args[0] {
	case "install":
		fs := flag.NewFlagSet("autostart install", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cfgPath := fs.String("c", "", "path to yaml config file")
		if err := fs.Parse(args[1:]); err != nil {
			return fmt.Errorf("parse autostart install args: %w", err)
		}
		if strings.TrimSpace(*cfgPath) == "" {
			return fmt.Errorf("autostart install requires -c <config.yaml>")
		}
		if err := manager.Install(*cfgPath); err != nil {
			return err
		}
		status, _ := manager.Status()
		logger.Info("autostart installed", "label", manager.Label(), "plist_path", status.PlistPath)
		return nil
	case "uninstall":
		if err := manager.Uninstall(); err != nil {
			return err
		}
		logger.Info("autostart uninstalled", "label", manager.Label())
		return nil
	case "status":
		status, err := manager.Status()
		if err != nil {
			return err
		}
		logger.Info(
			"autostart status",
			"label", status.Label,
			"installed", status.Installed,
			"loaded", status.Loaded,
			"plist_path", status.PlistPath,
		)
		return nil
	default:
		return fmt.Errorf("unknown autostart command: %s", args[0])
	}
}
