package adapter_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"anthropic-gateway/internal/adapter"
	"anthropic-gateway/internal/config"
)

func TestApplyAuthHeaders(t *testing.T) {
	ad := adapter.NewAnthropicCompatibleAdapter()
	h := http.Header{}
	h.Set("Authorization", "Bearer inbound")
	h.Set("x-api-key", "inbound")

	ad.ApplyAuthHeaders(h, config.UpstreamParams{APIKey: "target-key", AuthType: config.AuthTypeXAPIKey})
	if got := h.Get("x-api-key"); got != "target-key" {
		t.Fatalf("x-api-key = %q, want target-key", got)
	}
	if got := h.Get("Authorization"); got != "" {
		t.Fatalf("authorization should be removed for x-api-key mode, got %q", got)
	}

	ad.ApplyAuthHeaders(h, config.UpstreamParams{APIKey: "b-key", AuthType: config.AuthTypeBearer})
	if got := h.Get("Authorization"); got != "Bearer b-key" {
		t.Fatalf("authorization = %q, want Bearer b-key", got)
	}
	if got := h.Get("x-api-key"); got != "" {
		t.Fatalf("x-api-key should be removed for bearer mode, got %q", got)
	}
}

func TestBuildUpstreamURL(t *testing.T) {
	ad := adapter.NewAnthropicCompatibleAdapter()
	got, err := ad.BuildUpstreamURL("https://a.example.com/prefix", "/v1/messages", "x=1")
	if err != nil {
		t.Fatalf("build url: %v", err)
	}
	want := "https://a.example.com/prefix/v1/messages?x=1"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestNormalizeUpstreamError(t *testing.T) {
	ad := adapter.NewAnthropicCompatibleAdapter()

	anthropicBody := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)
	if got := ad.NormalizeUpstreamError(http.StatusBadRequest, anthropicBody, "req-1"); string(got) != string(anthropicBody) {
		t.Fatalf("anthropic payload should pass through")
	}

	normalized := ad.NormalizeUpstreamError(http.StatusBadGateway, []byte("backend failure"), "req-2")
	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("normalized should be json: %v", err)
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object")
	}
	if !strings.Contains(errObj["message"].(string), "backend failure") {
		t.Fatalf("unexpected error message: %v", errObj["message"])
	}
}
