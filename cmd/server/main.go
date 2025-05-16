package main

import (
	"context"
	"fmt"
	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
	"github.com/YuarenArt/GoBalancer/internal/server"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewLogger(cfg)
	srv, err := server.NewServer(cfg, logger)
	if err != nil {
		logger.Error("Failed to create server", "error", err)
		os.Exit(1)
	}

	// Create context that listens for interrupt signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for either a server error or context cancellation
	select {
	case err := <-errChan:
		if err != nil {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("Received shutdown signal")
	}

	logger.Info("Main process exiting")
}
