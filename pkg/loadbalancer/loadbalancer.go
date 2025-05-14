package loadbalancer

import (
	"errors"
	"github.com/YuarenArt/GoBalancer/internal/config"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

var (
	LeastConnBalancerType = "least_conn"
)

// Balancer defines the interface for load balancing algorithms.
// Implementations must select the next backend and optionally
// allow updating metrics used for selection.
type Balancer interface {
	// Next returns the address of the next backend to handle a request.
	Next() (*Backend, error)

	// MarkFailure marks the backend as down on error.
	MarkFailure(b *Backend)
}

// NewBalancer creates Balancer instances based on the configuration.
func NewBalancer(cfg *config.BalancerConfig) (Balancer, error) {
	backends := make([]*Backend, 0, len(cfg.Backends))
	for _, addr := range cfg.Backends {
		backend, err := NewBackend(addr)
		if err != nil {
			return nil, errors.New("failed to create backend: " + err.Error())
		}
		backends = append(backends, backend)
	}

	switch cfg.Type {
	case LeastConnBalancerType:
		return NewLeastConnBalancer(
			WithBackends(backends),
		), nil
	default:
		return nil, errors.New("unknown balancer type: " + cfg.Type)
	}
}

// Backend represents a single backend server.
type Backend struct {
	URL          *url.URL
	Alive        bool
	reverseProxy http.Handler
	mux          sync.RWMutex
}

// NewBackend creates a Backend with reverse proxy.
func NewBackend(rawURL string) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		if req.Context().Err() != nil {
			http.Error(w, "Request canceled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
	}

	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	return &Backend{
		URL:          u,
		Alive:        true,
		reverseProxy: proxy,
	}, nil
}

// ServeProxy forwards request to this backend.
func (b *Backend) ServeProxy(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, "Request canceled", http.StatusRequestTimeout)
		return
	}

	b.reverseProxy.ServeHTTP(w, r)
}

// IsAlive returns backend's alive status.
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.Alive
}

// SetAlive updates backend's alive status.
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.Alive = alive
}
