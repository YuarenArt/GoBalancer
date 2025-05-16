package loadbalancer

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// Backend represents a single backend server.
type Backend struct {
	URL          *url.URL
	Alive        bool
	reverseProxy http.Handler
	mux          sync.RWMutex
	failCount    int
	maxFails     int
	lastFailed   time.Time
}

// NewBackend creates a Backend with reverse proxy.
func NewBackend(rawURL string) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("invalid URL: the scheme must be http or https")
	}
	if u.Host == "" {
		return nil, errors.New("invalid URL: requires a host")
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
		maxFails:     3,
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

// SetAlive updates backend's alive status with failure threshold.
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	defer b.mux.Unlock()
	if alive {
		b.Alive = true
		b.failCount = 0
		b.lastFailed = time.Time{}
	} else {
		b.failCount++
		b.lastFailed = time.Now()
		if b.failCount >= b.maxFails {
			b.Alive = false
			b.failCount = b.maxFails
		}
	}
}
