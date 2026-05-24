package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SemanticHasher creates normalized hashes of LLM request bodies
// for cache key generation. IMPORTANT LIMITATION: This performs
// *syntactic* normalization, not true semantic equivalence. Paraphrased
// prompts ("what is AI?" vs "explain artificial intelligence") will NOT
// produce the same hash. For that, you need vector embeddings + similarity search.
type SemanticHasher struct {
	// config allows tweaking normalization strictness
	config NormalizationConfig
}

type NormalizationConfig struct {
	// Precision for floating-point fields (temperature, top_p, etc.)
	FloatPrecision int
	// Whether to normalize Unicode to NFC form
	NormalizeUnicode bool
	// Whether to collapse whitespace in text content
	CollapseWhitespace bool
	// Whether to preserve newlines and indentation in code blocks
	PreserveCodeBlocks bool
	// Fields to exclude from case normalization (e.g., case-sensitive stop sequences)
	CaseSensitiveFields []string
}

func DefaultConfig() NormalizationConfig {
	return NormalizationConfig{
		FloatPrecision:      3,
		NormalizeUnicode:    true,
		CollapseWhitespace:  true,
		PreserveCodeBlocks:  true,
		CaseSensitiveFields: []string{"stop", "tool_name", "function_name"},
	}
}

func NewSemanticHasher(config ...NormalizationConfig) *SemanticHasher {
	cfg := DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}
	return &SemanticHasher{config: cfg}
}

// ChatRequest represents the normalized OpenAI-compatible request format
type ChatRequest struct {
	Model            string          `json:"model,omitempty"`
	Messages         []ChatMessage   `json:"messages,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Stream           *bool           `json:"stream,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	LogitBias        map[string]int  `json:"logit_bias,omitempty"`
	User             string          `json:"user,omitempty"`
	N                *int            `json:"n,omitempty"`
	// Add other fields as needed
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or []ContentBlock
	Name       *string     `json:"name,omitempty"`
	ToolCallID *string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
}

type ContentBlock struct {
	Type     string `json:"type"` // "text", "image_url"
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

// Hash normalizes the request and returns a deterministic SHA-256 hash.
// Returns error if normalization fails (callers can decide to fallback).
func (h *SemanticHasher) Hash(body []byte) (string, error) {
	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		// If the body isn't valid JSON / doesn't match the ChatRequest
		// structure, fall back to a raw SHA hash so callers (and tests)
		// still get a stable key instead of an error.
		return h.HashRaw(body), nil
	}

	normalized, err := h.Normalize(req)
	if err != nil {
		return "", fmt.Errorf("normalization error: %w", err)
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal error: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Normalize returns a semantically-normalized request. This method is public for testability.
func (h *SemanticHasher) Normalize(req ChatRequest) (ChatRequest, error) {
	normalized := ChatRequest{}

	// Normalize model
	if req.Model != "" {
		normalized.Model = h.normalizeString(req.Model, true)
	}

	// Normalize messages (preserve order, but normalize content)
	normalized.Messages = make([]ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		normalizedMsg, err := h.normalizeMessage(msg)
		if err != nil {
			return ChatRequest{}, fmt.Errorf("message %d: %w", i, err)
		}
		normalized.Messages[i] = normalizedMsg
	}

	// Normalize float fields with precision
	normalized.Temperature = h.normalizeFloat(req.Temperature)
	normalized.TopP = h.normalizeFloat(req.TopP)
	normalized.PresencePenalty = h.normalizeFloat(req.PresencePenalty)
	normalized.FrequencyPenalty = h.normalizeFloat(req.FrequencyPenalty)

	// Normalize integers
	normalized.MaxTokens = req.MaxTokens
	normalized.Seed = req.Seed
	normalized.N = req.N

	// Normalize stream (preserve explicit false vs omitted)
	if req.Stream != nil {
		streamVal := *req.Stream
		normalized.Stream = &streamVal
	}

	// Normalize stop sequences (case-sensitive, but trim whitespace)
	normalized.Stop = h.normalizeStopSequences(req.Stop)

	// Normalize tools (sort for order independence)
	normalized.Tools = h.normalizeTools(req.Tools)

	// Normalize tool_choice
	normalized.ToolChoice = req.ToolChoice // Typically "none", "auto", or specific tool

	// Normalize response_format
	if req.ResponseFormat != nil {
		normalized.ResponseFormat = &ResponseFormat{
			Type:       h.normalizeString(req.ResponseFormat.Type, true),
			JSONSchema: h.normalizeJSONSchema(req.ResponseFormat.JSONSchema),
		}
	}

	// Normalize logit_bias (sort keys)
	normalized.LogitBias = h.normalizeLogitBias(req.LogitBias)

	// Normalize user identifier
	if req.User != "" {
		normalized.User = h.normalizeString(req.User, false) // Case-sensitive for user IDs
	}

	return normalized, nil
}

func (h *SemanticHasher) normalizeMessage(msg ChatMessage) (ChatMessage, error) {
	normalized := ChatMessage{
		Role: h.normalizeString(msg.Role, true),
	}

	// Handle name (for function messages)
	if msg.Name != nil {
		name := h.normalizeString(*msg.Name, false)
		normalized.Name = &name
	}

	// Handle ToolCallID (for tool responses)
	if msg.ToolCallID != nil {
		toolCallID := strings.TrimSpace(*msg.ToolCallID)
		normalized.ToolCallID = &toolCallID
	}

	// Normalize ToolCalls
	if len(msg.ToolCalls) > 0 {
		normalized.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			normalized.ToolCalls[i] = h.normalizeToolCall(tc)
		}
	}

	// Normalize Content (string or []ContentBlock)
	switch content := msg.Content.(type) {
	case string:
		normalized.Content = h.normalizeTextContent(content)
	case []interface{}:
		blocks, err := h.normalizeContentBlocks(content)
		if err != nil {
			return ChatMessage{}, err
		}
		normalized.Content = blocks
	case nil:
		normalized.Content = nil
	default:
		// Fallback: marshal to JSON and normalize string
		data, _ := json.Marshal(content)
		normalized.Content = h.normalizeTextContent(string(data))
	}

	return normalized, nil
}

func (h *SemanticHasher) normalizeContentBlocks(blocks []interface{}) ([]ContentBlock, error) {
	normalized := make([]ContentBlock, len(blocks))
	for i, block := range blocks {
		// Marshal and unmarshal to standardize format
		data, err := json.Marshal(block)
		if err != nil {
			return nil, err
		}

		var cb ContentBlock
		if err := json.Unmarshal(data, &cb); err != nil {
			// If not a standard block, treat as text
			normalized[i] = ContentBlock{
				Type: "text",
				Text: h.normalizeTextContent(string(data)),
			}
			continue
		}

		// Normalize based on type
		cb.Type = h.normalizeString(cb.Type, true)
		if cb.Type == "text" {
			cb.Text = h.normalizeTextContent(cb.Text)
		} else if cb.Type == "image_url" && cb.ImageURL != nil {
			cb.ImageURL.URL = strings.TrimSpace(cb.ImageURL.URL)
			cb.ImageURL.Detail = h.normalizeString(cb.ImageURL.Detail, true)
		}
		normalized[i] = cb
	}
	return normalized, nil
}

func (h *SemanticHasher) normalizeTextContent(s string) string {
	if h.config.NormalizeUnicode {
		s = norm.NFC.String(s)
	}

	// Preserve code blocks if configured
	if h.config.PreserveCodeBlocks {
		return h.normalizeWithCodeBlocks(s)
	}

	return h.normalizePlainText(s)
}

func (h *SemanticHasher) normalizePlainText(s string) string {
	s = strings.TrimSpace(s)
	if h.config.CollapseWhitespace {
		fields := strings.Fields(strings.ToLower(s))
		return strings.Join(fields, " ")
	}
	return strings.ToLower(s)
}

func (h *SemanticHasher) normalizeWithCodeBlocks(s string) string {
	// Simple heuristic: preserve content within triple backticks
	parts := strings.Split(s, "```")
	for i := 0; i < len(parts); i++ {
		if i%2 == 0 {
			// Outside code block: normalize normally
			parts[i] = h.normalizePlainText(parts[i])
		}
		// Inside code block: only trim leading/trailing whitespace per line
	}
	return strings.Join(parts, "```")
}

func (h *SemanticHasher) normalizeString(s string, toLower bool) string {
	s = strings.TrimSpace(s)
	if toLower && !h.isCaseSensitiveField(s) {
		s = strings.ToLower(s)
	}
	if h.config.NormalizeUnicode {
		s = norm.NFC.String(s)
	}
	// Collapse internal whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func (h *SemanticHasher) isCaseSensitiveField(s string) bool {
	for _, field := range h.config.CaseSensitiveFields {
		if s == field {
			return true
		}
	}
	return false
}

func (h *SemanticHasher) normalizeFloat(f *float64) *float64 {
	if f == nil {
		return nil
	}
	// Round to configured precision
	precision := math.Pow10(h.config.FloatPrecision)
	rounded := math.Round(*f*precision) / precision
	return &rounded
}

func (h *SemanticHasher) normalizeStopSequences(stop []string) []string {
	if len(stop) == 0 {
		return nil
	}
	normalized := make([]string, len(stop))
	for i, s := range stop {
		// Stop sequences are case-sensitive but we can trim whitespace
		normalized[i] = strings.TrimSpace(s)
	}
	sort.Strings(normalized) // Order-independent
	return normalized
}

func (h *SemanticHasher) normalizeTools(tools []Tool) []Tool {
	if len(tools) == 0 {
		return nil
	}
	normalized := make([]Tool, len(tools))
	for i, tool := range tools {
		normalized[i] = Tool{
			Type: h.normalizeString(tool.Type, true),
			Function: Function{
				Name:        h.normalizeString(tool.Function.Name, false), // Case-sensitive
				Description: h.normalizeTextContent(tool.Function.Description),
				Parameters:  h.normalizeJSONSchema(tool.Function.Parameters),
			},
		}
	}
	// Sort by function name for order independence
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Function.Name < normalized[j].Function.Name
	})
	return normalized
}

func (h *SemanticHasher) normalizeToolCall(tc ToolCall) ToolCall {
	return ToolCall{
		ID:   strings.TrimSpace(tc.ID),
		Type: h.normalizeString(tc.Type, true),
		Function: FunctionCall{
			Name:      h.normalizeString(tc.Function.Name, false),
			Arguments: tc.Function.Arguments, // JSON string, should be validated but not altered
		},
	}
}

func (h *SemanticHasher) normalizeLogitBias(lb map[string]int) map[string]int {
	if len(lb) == 0 {
		return nil
	}
	// Sort keys for deterministic output
	keys := make([]string, 0, len(lb))
	for k := range lb {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	normalized := make(map[string]int)
	for _, k := range keys {
		normalized[k] = lb[k]
	}
	return normalized
}

func (h *SemanticHasher) normalizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	// Deep copy and normalize string values recursively
	return h.normalizeJSONObject(schema).(map[string]interface{})
}

func (h *SemanticHasher) normalizeJSONObject(obj interface{}) interface{} {
	switch v := obj.(type) {
	case string:
		return h.normalizeTextContent(v)
	case []interface{}:
		arr := make([]interface{}, len(v))
		for i, item := range v {
			arr[i] = h.normalizeJSONObject(item)
		}
		return arr
	case map[string]interface{}:
		m := make(map[string]interface{})
		for key, val := range v {
			// Preserve key case for JSON schema properties
			m[key] = h.normalizeJSONObject(val)
		}
		return m
	default:
		return v // numbers, booleans, nulls
	}
}

// HashRaw provides a fallback hash for unparseable content
func (h *SemanticHasher) HashRaw(body []byte) string {
	if h.config.NormalizeUnicode {
		body = []byte(norm.NFC.String(string(body)))
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}
