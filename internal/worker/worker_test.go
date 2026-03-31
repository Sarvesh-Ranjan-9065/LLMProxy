package worker

import (
    "testing"
    "time"
)

func TestProcessor(t *testing.T) {
    // Use short delays for testing
    p := NewProcessor(10*time.Millisecond, 50*time.Millisecond)

    messages := []Message{
        {Role: "user", Content: "What is Go?"},
    }

    start := time.Now()
    content, tokens := p.Process("test-model", messages)
    elapsed := time.Since(start)

    if content == "" {
        t.Error("response should not be empty")
    }
    if tokens <= 0 {
        t.Error("token count should be positive")
    }
    if elapsed < 10*time.Millisecond {
        t.Error("should have some processing delay")
    }

    t.Logf("✅ Response: %s...", content[:50])
    t.Logf("✅ Tokens: %d", tokens)
    t.Logf("✅ Processing time: %v", elapsed)
}

func TestBuildResponse(t *testing.T) {
    resp := BuildResponse("gpt-3.5-turbo", "Hello there!", 10)

    if resp.Object != "chat.completion" {
        t.Errorf("expected chat.completion, got %s", resp.Object)
    }
    if len(resp.Choices) != 1 {
        t.Errorf("expected 1 choice, got %d", len(resp.Choices))
    }
    if resp.Choices[0].Message.Role != "assistant" {
        t.Error("response role should be assistant")
    }
    if resp.Usage.PromptTokens != 10 {
        t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
    }
    if resp.ID == "" {
        t.Error("ID should not be empty")
    }

    t.Logf("✅ Response ID: %s", resp.ID)
    t.Logf("✅ Model: %s", resp.Model)
    t.Logf("✅ Tokens: prompt=%d completion=%d total=%d",
        resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
}
