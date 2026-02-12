package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"anthropic-gateway/internal/adapter"
	"anthropic-gateway/internal/config"
	apierrors "anthropic-gateway/internal/errors"
)

const (
	contextKeyRequestID = "request_id"
)

type Service struct {
	cfg     *config.Config
	adapter adapter.Adapter
	client  *http.Client
	logger  *slog.Logger
}

func NewService(cfg *config.Config, ad adapter.Adapter, logger *slog.Logger) *Service {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{Transport: transport}

	return &Service{
		cfg:     cfg,
		adapter: ad,
		client:  client,
		logger:  logger,
	}
}

func (s *Service) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	s.proxyJSON(w, r)
}

func (s *Service) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	s.proxyJSON(w, r)
}

func (s *Service) HandleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, http.MethodGet)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.adapter.BuildModelsResponse(s.cfg)); err != nil {
		s.logger.Error("failed to encode models response", "error", err, "request_id", requestIDFromContext(r.Context()))
		apierrors.Write(w, http.StatusInternalServerError, "api_error", "failed to encode response", requestIDFromContext(r.Context()))
		return
	}
}

func (s *Service) HandleUnsupported(w http.ResponseWriter, r *http.Request) {
	apierrors.Write(
		w,
		http.StatusNotFound,
		"not_found_error",
		"path is not supported by this gateway",
		requestIDFromContext(r.Context()),
	)
}

func (s *Service) proxyJSON(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierrors.Write(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request body too large", requestID)
			return
		}
		apierrors.Write(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body", requestID)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		apierrors.Write(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON payload", requestID)
		return
	}

	requestedModel, ok := payload["model"].(string)
	if !ok || strings.TrimSpace(requestedModel) == "" {
		apierrors.Write(w, http.StatusBadRequest, "invalid_request_error", "model is required", requestID)
		return
	}

	route, found := s.cfg.RouteByModel(requestedModel)
	if !found {
		apierrors.Write(w, http.StatusBadRequest, "invalid_request_error", "unknown model: "+requestedModel, requestID)
		return
	}

	payload["model"] = route.Params.Model
	mutatedBody, err := json.Marshal(payload)
	if err != nil {
		apierrors.Write(w, http.StatusBadRequest, "invalid_request_error", "failed to marshal request payload", requestID)
		return
	}

	upstreamPath := strings.TrimPrefix(r.URL.Path, "/anthropic")
	upstreamURL, err := s.adapter.BuildUpstreamURL(route.Params.APIBase, upstreamPath, r.URL.RawQuery)
	if err != nil {
		s.logger.Error("failed to build upstream URL", "error", err, "request_id", requestID)
		apierrors.Write(w, http.StatusInternalServerError, "api_error", "failed to build upstream request", requestID)
		return
	}

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(mutatedBody))
	if err != nil {
		s.logger.Error("failed to build upstream request", "error", err, "request_id", requestID)
		apierrors.Write(w, http.StatusInternalServerError, "api_error", "failed to build upstream request", requestID)
		return
	}

	copyRequestHeaders(upReq.Header, r.Header)
	s.adapter.ApplyAuthHeaders(upReq.Header, route.Params)
	if upReq.Header.Get("Content-Type") == "" {
		upReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(upReq)
	if err != nil {
		s.handleUpstreamFailure(w, err, requestID)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)

	if isEventStream(resp.Header) {
		w.WriteHeader(resp.StatusCode)
		s.streamResponse(w, resp.Body, requestID)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Error("failed to read upstream response", "error", err, "request_id", requestID)
		apierrors.Write(w, http.StatusBadGateway, "api_error", "failed to read upstream response", requestID)
		return
	}

	if resp.StatusCode >= http.StatusBadRequest {
		normalized := s.adapter.NormalizeUpstreamError(resp.StatusCode, respBody, requestID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(normalized)
		return
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (s *Service) handleUpstreamFailure(w http.ResponseWriter, err error, requestID string) {
	s.logger.Error("upstream request failed", "error", err, "request_id", requestID)

	if errors.Is(err, context.DeadlineExceeded) {
		apierrors.Write(w, http.StatusGatewayTimeout, "api_error", "upstream timeout", requestID)
		return
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		apierrors.Write(w, http.StatusGatewayTimeout, "api_error", "upstream timeout", requestID)
		return
	}

	apierrors.Write(w, http.StatusBadGateway, "api_error", "upstream request failed", requestID)
}

func (s *Service) streamResponse(w http.ResponseWriter, body io.Reader, requestID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return
	}

	buf := make([]byte, 4*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				s.logger.Error("failed to write stream chunk", "error", writeErr, "request_id", requestID)
				return
			}
			flusher.Flush()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			s.logger.Error("failed to read stream chunk", "error", err, "request_id", requestID)
			return
		}
	}
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allow string) {
	w.Header().Set("Allow", allow)
	apierrors.Write(
		w,
		http.StatusMethodNotAllowed,
		"invalid_request_error",
		"method not allowed",
		requestIDFromContext(r.Context()),
	)
}

func isEventStream(headers http.Header) bool {
	contentType := strings.ToLower(headers.Get("Content-Type"))
	return strings.Contains(contentType, "text/event-stream")
}

func copyRequestHeaders(dst, src http.Header) {
	for k, values := range src {
		if isHopByHopHeader(k) {
			continue
		}
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for k, values := range src {
		if isHopByHopHeader(k) {
			continue
		}
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func requestIDFromContext(ctx context.Context) string {
	v := ctx.Value(contextKeyRequestID)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKeyRequestID, requestID)
}
