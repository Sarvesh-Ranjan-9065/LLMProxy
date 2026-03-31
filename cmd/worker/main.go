package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/yourusername/llmproxy/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("WORKER_PORT")
	if port == "" {
		port = "9001"
	}

	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		workerID = "worker-" + port
	}

	ws := worker.NewWorkerServer(port, workerID)
	if err := ws.Start(); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}