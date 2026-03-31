package cache

import (
    "testing"
)

func TestHashIdenticalPrompts(t *testing.T) {
    h := NewSemanticHasher()

    body1 := []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello world"}]}`)
    body2 := []byte(`{"model":"GPT-3.5-Turbo","messages":[{"role":"user","content":"  hello   world  "}]}`)

    hash1, _ := h.Hash(body1)
    hash2, _ := h.Hash(body2)

    if hash1 != hash2 {
        t.Errorf("identical prompts should hash the same\nhash1: %s\nhash2: %s", hash1, hash2)
    }
    t.Logf("✅ Same prompt, different formatting → same hash: %s", hash1[:16]+"...")
}

func TestHashDifferentPrompts(t *testing.T) {
    h := NewSemanticHasher()

    body1 := []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}`)
    body2 := []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Goodbye"}]}`)

    hash1, _ := h.Hash(body1)
    hash2, _ := h.Hash(body2)

    if hash1 == hash2 {
        t.Error("different prompts should NOT hash the same")
    }
    t.Logf("✅ Different prompts → different hashes")
}

func TestMessageOrderPreserved(t *testing.T) {
    h := NewSemanticHasher()

    // Order A: system then user
    bodyA := []byte(`{"model":"gpt-3.5-turbo","messages":[
        {"role":"system","content":"You are helpful"},
        {"role":"user","content":"Hello"}
    ]}`)

    // Order B: user then system (different conversation!)
    bodyB := []byte(`{"model":"gpt-3.5-turbo","messages":[
        {"role":"user","content":"Hello"},
        {"role":"system","content":"You are helpful"}
    ]}`)

    hashA, _ := h.Hash(bodyA)
    hashB, _ := h.Hash(bodyB)

    if hashA == hashB {
        t.Error("different message order should produce different hashes")
    }
    t.Logf("✅ Message order preserved — different order = different hash")
}

func TestInvalidJSON(t *testing.T) {
    h := NewSemanticHasher()
    hash, err := h.Hash([]byte(`not json at all`))

    if err != nil {
        t.Errorf("should not error on invalid JSON: %v", err)
    }
    if hash == "" {
        t.Error("should still produce a hash for invalid JSON")
    }
    t.Logf("✅ Invalid JSON handled gracefully, raw hash: %s", hash[:16]+"...")
}
