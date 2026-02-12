package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"anthropic-gateway/internal/gateway"
)

var requestSeq uint64

type Server struct {
	httpServer *http.Server
}

func New(addr string, logger *slog.Logger, service *gateway.Service) *Server {
	handler := NewHandler(logger, service)
	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

func NewHandler(logger *slog.Logger, service *gateway.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/anthropic/v1/messages", service.HandleMessages)
	mux.HandleFunc("/anthropic/v1/messages/count_tokens", service.HandleCountTokens)
	mux.HandleFunc("/anthropic/v1/models", service.HandleModels)
	mux.HandleFunc("/anthropic", service.HandleUnsupported)
	mux.HandleFunc("/anthropic/", service.HandleUnsupported)

	handler := withRequestID(withLogging(mux, logger))
	return handler
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("x-request-id"))
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("x-request-id", requestID)

		ctx := gateway.ContextWithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		requestID := rw.Header().Get("x-request-id")
		logger.Info(
			"http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", requestID,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func generateRequestID() string {
	next := atomic.AddUint64(&requestSeq, 1)
	return fmt.Sprintf("req-%d-%s", next, strconv.FormatInt(rand.New(rand.NewSource(time.Now().UnixNano())).Int63(), 36))
}
