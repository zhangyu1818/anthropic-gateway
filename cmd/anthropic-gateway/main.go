package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"anthropic-gateway/internal/adapter"
	"anthropic-gateway/internal/config"
	"anthropic-gateway/internal/gateway"
	"anthropic-gateway/internal/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	args := os.Args[1:]

	var err error
	if len(args) > 0 && args[0] == "autostart" {
		err = runAutostart(args[1:], logger)
	} else {
		err = runGateway(args, logger)
	}

	if err == nil {
		return
	}

	logger.Error("command failed", "error", err)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	os.Exit(1)
}

func runGateway(args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("anthropic-gateway", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfgPath := fs.String("c", "", "path to yaml config file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse args: %w", err)
	}

	if strings.TrimSpace(*cfgPath) == "" {
		return fmt.Errorf("missing required -c <config.yaml>")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ad := adapter.NewAnthropicCompatibleAdapter()
	service := gateway.NewService(cfg, ad, logger)
	server := httpserver.New(cfg.Listen, logger, service)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway starting", "listen", cfg.Listen)
		errCh <- server.ListenAndServe()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server exited unexpectedly: %w", err)
		}
		return nil
	}

	if err := server.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	logger.Info("gateway stopped")
	return nil
}
