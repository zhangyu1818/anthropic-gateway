package adapter

import (
	"net/http"

	"anthropic-gateway/internal/config"
	"anthropic-gateway/internal/models"
)

type Adapter interface {
	BuildUpstreamURL(apiBase, upstreamPath, rawQuery string) (string, error)
	ApplyAuthHeaders(headers http.Header, params config.UpstreamParams)
	BuildModelsResponse(cfg *config.Config) models.ListResponse
	NormalizeUpstreamError(statusCode int, upstreamBody []byte, requestID string) []byte
}
