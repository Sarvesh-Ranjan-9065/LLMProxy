package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/yourusername/llmproxy/internal/config"
	"github.com/yourusername/llmproxy/internal/proxy"
)

func main() {
	// Set up structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Loading configuration...")
	cfg := config.Load()

	slog.Info("Initializing LLMProxy gateway...")
	server, err := proxy.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}