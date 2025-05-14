package loadbalancer

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
)

// LeastConnBalancer distributes load using Least Connections algorithm.
type LeastConnBalancer struct {
	backends []*Backend
	loads    map[*Backend]int
	mux      sync.RWMutex
}

// Option allows customizing the LeastConnBalancer.
type Option func(*LeastConnBalancer)

// WithBackends sets the backends for the balancer.
func WithBackends(backends []*Backend) Option {
	return func(lb *LeastConnBalancer) {
		lb.backends = backends
	}
}

// WithInitialLoads sets the initial load counts for the backends.
func WithInitialLoads(loads map[*Backend]int) Option {
	return func(lb *LeastConnBalancer) {
		lb.loads = loads
	}
}

// NewLeastConnBalancer creates a new LeastConnBalancer with the specified options.
func NewLeastConnBalancer(options ...Option) *LeastConnBalancer {
	// Initialize with default values
	lb := &LeastConnBalancer{
		backends: make([]*Backend, 0),
		loads:    make(map[*Backend]int),
	}

	// Apply provided options
	for _, opt := range options {
		opt(lb)
	}

	// Initialize loads for backends if not provided
	if len(lb.loads) == 0 && len(lb.backends) > 0 {
		for _, b := range lb.backends {
			lb.loads[b] = 0
		}
	}

	if err := lb.validate(); err != nil {
		panic(err.Error())
	}

	return lb
}

// Next selects the backend with the least number of active connections.
func (lb *LeastConnBalancer) Next() (*Backend, error) {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	var selected *Backend
	minConns := math.MaxInt32

	for _, b := range lb.backends {
		if !b.IsAlive() {
			continue
		}
		cnt := lb.loads[b]
		if cnt < minConns {
			minConns = cnt
			selected = b
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("no alive backends")
	}

	lb.loads[selected]++
	return selected, nil
}

// ServeHTTP handles incoming HTTP requests using least-connections balancing.
func (lb *LeastConnBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if err := r.Context().Err(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	backend, err := lb.Next()
	if err != nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer lb.Release(backend)
	backend.ServeProxy(w, r)
}

// Release decrements the load counter for the backend after request is done.
func (lb *LeastConnBalancer) Release(b *Backend) {
	lb.mux.Lock()
	defer lb.mux.Unlock()
	if lb.loads[b] > 0 {
		lb.loads[b]--
	}
}

// MarkFailure marks a backend as dead and resets its connection count.
func (lb *LeastConnBalancer) MarkFailure(b *Backend) {
	b.SetAlive(false)
	lb.mux.Lock()
	defer lb.mux.Unlock()
	lb.loads[b] = 0
}

// validate ensures the LeastConnBalancer configuration is valid.
func (lb *LeastConnBalancer) validate() error {
	if len(lb.backends) == 0 {
		return errors.New("at least one backend is required")
	}
	for _, b := range lb.backends {
		if _, exists := lb.loads[b]; !exists {
			return errors.New("load count missing for backend")
		}
	}
	return nil
}
