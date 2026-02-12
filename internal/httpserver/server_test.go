package httpserver_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anthropic-gateway/internal/adapter"
	"anthropic-gateway/internal/config"
	"anthropic-gateway/internal/gateway"
	"anthropic-gateway/internal/httpserver"
)

func TestMessagesNonStreamingSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer target-key" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Fatalf("x-api-key should be empty, got %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if got := payload["model"].(string); got != "glm-4.7" {
			t.Fatalf("model = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message"}`))
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL)
	defer gw.Close()

	body := []byte(`{"model":"sonnet","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	req, err := http.NewRequest(http.MethodPost, gw.URL+"/anthropic/v1/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer inbound")
	req.Header.Set("x-api-key", "inbound")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "msg_1") {
		t.Fatalf("unexpected response body: %s", string(respBody))
	}
}

func TestMessagesStreamingSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL)
	defer gw.Close()

	body := []byte(`{"model":"sonnet","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	resp, err := http.Post(gw.URL+"/anthropic/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}

	respBody, _ := io.ReadAll(resp.Body)
	out := string(respBody)
	if !strings.Contains(out, "message_start") || !strings.Contains(out, "message_stop") {
		t.Fatalf("unexpected stream body: %s", out)
	}
}

func TestMessagesStreamingFlushesBeforeCompletion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		time.Sleep(700 * time.Millisecond)

		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL)
	defer gw.Close()

	start := time.Now()
	body := []byte(`{"model":"sonnet","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	resp, err := http.Post(gw.URL+"/anthropic/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first stream line: %v", err)
	}
	firstChunkDelay := time.Since(start)

	if !strings.Contains(firstLine, "event: message_start") {
		t.Fatalf("unexpected first stream line: %q", firstLine)
	}
	if firstChunkDelay >= 500*time.Millisecond {
		t.Fatalf("first chunk arrived too late, delay=%v", firstChunkDelay)
	}

	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read remaining stream: %v", err)
	}
	total := time.Since(start)
	if !strings.Contains(string(rest), "message_stop") {
		t.Fatalf("missing message_stop in stream")
	}
	if total < 650*time.Millisecond {
		t.Fatalf("total duration too short for delayed second chunk, total=%v", total)
	}
}

func TestCountTokensSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"input_tokens":42}`))
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL)
	defer gw.Close()

	body := []byte(`{"model":"sonnet","messages":[{"role":"user","content":"hi"}]}`)
	resp, err := http.Post(gw.URL+"/anthropic/v1/messages/count_tokens", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "input_tokens") {
		t.Fatalf("unexpected body: %s", string(respBody))
	}
}

func TestModelsFromConfig(t *testing.T) {
	gw := newGatewayServer(t, "https://example.com")
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/anthropic/v1/models")
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "sonnet") {
		t.Fatalf("models response missing model: %s", string(body))
	}
}

func TestUnknownModelReturns400(t *testing.T) {
	gw := newGatewayServer(t, "https://example.com")
	defer gw.Close()

	resp, err := http.Post(gw.URL+"/anthropic/v1/messages", "application/json", strings.NewReader(`{"model":"unknown"}`))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "unknown model") {
		t.Fatalf("unexpected error body: %s", string(body))
	}
}

func TestUnsupportedPathReturns404(t *testing.T) {
	gw := newGatewayServer(t, "https://example.com")
	defer gw.Close()

	resp, err := http.Post(gw.URL+"/anthropic/v1/messages/batches", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestUpstreamPlainTextErrorIsNormalized(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend broken", http.StatusBadGateway)
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL)
	defer gw.Close()

	resp, err := http.Post(gw.URL+"/anthropic/v1/messages", "application/json", strings.NewReader(`{"model":"sonnet"}`))
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"type":"error"`) || !strings.Contains(string(body), "backend broken") {
		t.Fatalf("unexpected normalized body: %s", string(body))
	}
}

func newGatewayServer(t *testing.T, upstreamURL string) *httptest.Server {
	t.Helper()

	cfg := &config.Config{
		Listen: ":0",
		ModelList: []config.ModelRoute{
			{
				ModelName: "sonnet",
				Params: config.UpstreamParams{
					Model:    "glm-4.7",
					APIBase:  upstreamURL,
					APIKey:   "target-key",
					AuthType: config.AuthTypeBearer,
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := gateway.NewService(cfg, adapter.NewAnthropicCompatibleAdapter(), logger)
	handler := httpserver.NewHandler(logger, service)
	return httptest.NewServer(handler)
}
