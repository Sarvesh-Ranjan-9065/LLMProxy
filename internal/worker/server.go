package worker

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// WorkerServer is the mock LLM worker HTTP server
type WorkerServer struct {
	processor *Processor
	port      string
	workerID  string
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

func NewWorkerServer(port, workerID string) *WorkerServer {
	return &WorkerServer{
		processor: NewProcessor(200*time.Millisecond, 3*time.Second),
		port:      port,
		workerID:  workerID,
	}
}

func (ws *WorkerServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", ws.handleChatCompletion)
	mux.HandleFunc("/v1/completions", ws.handleChatCompletion)
	mux.HandleFunc("/health", ws.handleHealth)

	server := &http.Server{
		Addr:         ":" + ws.port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	slog.Info("Mock LLM worker starting",
		"worker_id", ws.workerID,
		"port", ws.port,
	)
	return server.ListenAndServe()
}

func (ws *WorkerServer) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		req.Model = "mock-gpt-3.5-turbo"
	}

	// Convert messages to internal format
	messages := make([]Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = Message{Role: m.Role, Content: m.Content}
	}

	start := time.Now()
	content, promptTokens := ws.processor.Process(req.Model, messages)
	duration := time.Since(start)

	response := BuildResponse(req.Model, content, promptTokens)

	slog.Info("request processed",
		"worker_id", ws.workerID,
		"model", req.Model,
		"duration_ms", duration.Milliseconds(),
		"prompt_tokens", promptTokens,
	)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Worker-ID", ws.workerID)
	w.Header().Set("X-Processing-Time", duration.String())
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (ws *WorkerServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"worker_id": ws.workerID,
		"timestamp": time.Now().Unix(),
	})
}