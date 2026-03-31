package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// SemanticHasher creates normalized hashes of LLM request bodies
// so that semantically identical prompts produce the same cache key.
type SemanticHasher struct{}

func NewSemanticHasher() *SemanticHasher {
	return &SemanticHasher{}
}

// ChatRequest represents the OpenAI-compatible request format
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Hash normalizes the request body and returns a SHA-256 hash.
// The returned hash does NOT include any tenant/key prefix — callers
// must namespace the key themselves to guarantee tenant isolation.
func (h *SemanticHasher) Hash(body []byte) (string, error) {
	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		// If we can't parse it as a chat request, hash the raw body
		return h.hashRaw(body), nil
	}

	normalized := h.normalize(req)

	data, err := json.Marshal(normalized)
	if err != nil {
		return h.hashRaw(body), nil
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func (h *SemanticHasher) normalize(req ChatRequest) ChatRequest {
	normalized := ChatRequest{
		Model:       strings.ToLower(strings.TrimSpace(req.Model)),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
	}

	// Preserve original message order — conversation order is semantically
	// meaningful.  Only normalise text formatting (case, whitespace).
	messages := make([]ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = ChatMessage{
			Role:    strings.ToLower(strings.TrimSpace(msg.Role)),
			Content: normalizeText(msg.Content),
		}
	}

	normalized.Messages = messages
	return normalized
}

func normalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func (h *SemanticHasher) hashRaw(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}