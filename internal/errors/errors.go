package apierrors

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Envelope struct {
	Type      string `json:"type"`
	Error     Inner  `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type Inner struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func Marshal(errorType, message, requestID string) []byte {
	if strings.TrimSpace(message) == "" {
		message = "request failed"
	}
	payload := Envelope{
		Type: "error",
		Error: Inner{
			Type:    errorType,
			Message: message,
		},
		RequestID: requestID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"type":"error","error":{"type":"api_error","message":"failed to marshal error"}}`)
	}
	return body
}

func Write(w http.ResponseWriter, statusCode int, errorType, message, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(Marshal(errorType, message, requestID))
}

func IsAnthropicErrorPayload(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var payload Envelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if payload.Type != "error" {
		return false
	}
	if strings.TrimSpace(payload.Error.Type) == "" || strings.TrimSpace(payload.Error.Message) == "" {
		return false
	}
	return true
}
