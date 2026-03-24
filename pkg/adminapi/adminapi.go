package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/example/load-balancer/pkg/backendpool"
	"github.com/example/load-balancer/pkg/config"
	"github.com/example/load-balancer/pkg/metrics"
	"github.com/example/load-balancer/pkg/strategies"
)

type API struct {
	pool    *backendpool.Pool
	metrics *metrics.Registry
}

func New(pool *backendpool.Pool, registry *metrics.Registry) *API {
	return &API{pool: pool, metrics: registry}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/backends", a.backendsHandler)
	mux.HandleFunc("/admin/backends/", a.backendByIDHandler)
	mux.HandleFunc("/admin/strategy", a.strategyHandler)
	mux.HandleFunc("/admin/state", a.stateHandler)
	mux.Handle("/metrics", a.metrics.Handler())
	return mux
}

func (a *API) backendsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.writeJSON(w, http.StatusOK, a.pool.List())
	case http.MethodPost:
		var req config.BackendConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if req.ID == "" || req.Host == "" || req.Port <= 0 {
			a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id, host, and port are required"})
			return
		}
		if err := a.pool.AddBackend(req); err != nil {
			a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		a.writeJSON(w, http.StatusCreated, map[string]string{"status": "backend added"})
	default:
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) backendByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/backends/")
	if id == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing backend id"})
		return
	}
	if err := a.pool.RemoveBackend(id); err != nil {
		a.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "backend removed"})
}

func (a *API) strategyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.writeJSON(w, http.StatusOK, map[string]string{"strategy": a.pool.StrategyName()})
	case http.MethodPost:
		var req struct {
			Strategy string `json:"strategy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		s, err := strategies.New(req.Strategy)
		if err != nil {
			a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		a.pool.SetStrategy(s)
		a.writeJSON(w, http.StatusOK, map[string]string{"status": "strategy updated", "strategy": s.Name()})
	default:
		a.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) stateHandler(w http.ResponseWriter, _ *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]any{
		"strategy": a.pool.StrategyName(),
		"backends": a.pool.List(),
		"metrics":  a.metrics.Snapshot(),
	})
}

func (a *API) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
