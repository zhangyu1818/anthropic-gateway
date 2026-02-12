package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"anthropic-gateway/internal/config"
	apierrors "anthropic-gateway/internal/errors"
	"anthropic-gateway/internal/models"
)

type AnthropicCompatibleAdapter struct{}

func NewAnthropicCompatibleAdapter() *AnthropicCompatibleAdapter {
	return &AnthropicCompatibleAdapter{}
}

func (a *AnthropicCompatibleAdapter) BuildUpstreamURL(apiBase, upstreamPath, rawQuery string) (string, error) {
	base, err := url.Parse(apiBase)
	if err != nil {
		return "", fmt.Errorf("parse api_base: %w", err)
	}

	suffix := "/" + strings.TrimLeft(upstreamPath, "/")
	base.Path = strings.TrimRight(base.Path, "/") + suffix
	base.RawQuery = rawQuery
	return base.String(), nil
}

func (a *AnthropicCompatibleAdapter) ApplyAuthHeaders(headers http.Header, params config.UpstreamParams) {
	headers.Del("Authorization")
	headers.Del("x-api-key")
	switch params.AuthType {
	case config.AuthTypeBearer:
		headers.Set("Authorization", "Bearer "+params.APIKey)
	default:
		headers.Set("x-api-key", params.APIKey)
	}
}

func (a *AnthropicCompatibleAdapter) BuildModelsResponse(cfg *config.Config) models.ListResponse {
	return models.BuildListResponse(cfg)
}

func (a *AnthropicCompatibleAdapter) NormalizeUpstreamError(statusCode int, upstreamBody []byte, requestID string) []byte {
	if apierrors.IsAnthropicErrorPayload(upstreamBody) {
		return upstreamBody
	}

	message := extractMessage(upstreamBody)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	errorType := "invalid_request_error"
	if statusCode >= 500 {
		errorType = "api_error"
	}
	return apierrors.Marshal(errorType, message, requestID)
}

func extractMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}

	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		return trimmed
	}

	if msg, ok := generic["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}

	if errObj, ok := generic["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}

	return trimmed
}
