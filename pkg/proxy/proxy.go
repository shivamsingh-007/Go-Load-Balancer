package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/load-balancer/pkg/backendpool"
	"github.com/example/load-balancer/pkg/metrics"
)

type Handler struct {
	pool          *backendpool.Pool
	metrics       *metrics.Registry
	logger        *slog.Logger
	requestTimout time.Duration
	retryCount    int
	transport     *http.Transport
}

func NewHandler(pool *backendpool.Pool, m *metrics.Registry, logger *slog.Logger, requestTimeout time.Duration, retryCount int) *Handler {
	if retryCount < 0 {
		retryCount = 0
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          50000,
		MaxIdleConnsPerHost:   20000,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Handler{
		pool:          pool,
		metrics:       m,
		logger:        logger,
		requestTimout: requestTimeout,
		retryCount:    retryCount,
		transport:     transport,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.metrics.IncRequest()

	clientIP := extractClientIP(r)
	maxAttempts := 1
	if isIdempotent(r.Method) {
		maxAttempts += h.retryCount
	}
	excluded := map[string]bool{}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		backend, err := h.pool.SelectBackend(clientIP, excluded)
		if err != nil {
			h.respondError(w, http.StatusServiceUnavailable, "no healthy backends available")
			h.metrics.IncError()
			h.metrics.ObserveStatus(http.StatusServiceUnavailable)
			h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err.Error())
			return
		}

		status, err := h.forward(r, w, backend, clientIP)
		latency := time.Since(start)
		if err == nil {
			h.metrics.ObserveStatus(status)
			h.logger.Info("request proxied",
				"method", r.Method,
				"path", r.URL.Path,
				"backend_id", backend.ID,
				"backend_addr", backend.Address(),
				"status", status,
				"latency_ms", latency.Milliseconds(),
			)
			return
		}

		excluded[backend.ID] = true
		lastErr = err
		h.logger.Warn("attempt failed",
			"method", r.Method,
			"path", r.URL.Path,
			"backend_id", backend.ID,
			"attempt", attempt,
			"error", err.Error(),
		)
	}

	h.metrics.IncError()
	if isTimeoutErr(lastErr) {
		h.metrics.IncTimeout()
	}
	h.metrics.ObserveStatus(http.StatusBadGateway)
	h.respondError(w, http.StatusBadGateway, "all backend attempts failed")
}

func (h *Handler) forward(in *http.Request, outWriter http.ResponseWriter, backend *backendpool.Backend, clientIP string) (int, error) {
	h.pool.IncActive(backend.ID)
	h.metrics.IncBackendActive(backend.ID)
	defer func() {
		h.pool.DecActive(backend.ID)
		h.metrics.DecBackendActive(backend.ID)
	}()

	ctx, cancel := context.WithTimeout(in.Context(), h.requestTimout)
	defer cancel()

	targetURL, _ := url.Parse(backend.URL())
	outReq := in.Clone(ctx)
	outReq.URL.Scheme = targetURL.Scheme
	outReq.URL.Host = targetURL.Host
	outReq.Host = targetURL.Host
	outReq.RequestURI = ""
	outReq.Header = cloneHeader(in.Header)
	appendForwardedFor(outReq.Header, clientIP)

	resp, err := h.transport.RoundTrip(outReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			h.metrics.IncTimeout()
		}
		return 0, err
	}
	defer resp.Body.Close()

	copyHeader(outWriter.Header(), resp.Header)
	outWriter.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(outWriter, resp.Body)
	if copyErr != nil {
		return 0, copyErr
	}

	h.metrics.IncBackendRequest(backend.ID)
	return resp.StatusCode, nil
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func isIdempotent(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func extractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func appendForwardedFor(h http.Header, ip string) {
	current := h.Get("X-Forwarded-For")
	if current == "" {
		h.Set("X-Forwarded-For", ip)
		return
	}
	h.Set("X-Forwarded-For", current+", "+ip)
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, vv := range src {
		cp := make([]string, len(vv))
		copy(cp, vv)
		dst[k] = cp
	}
	return dst
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	type timeout interface{ Timeout() bool }
	te, ok := err.(timeout)
	return ok && te.Timeout()
}
