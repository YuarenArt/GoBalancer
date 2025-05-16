package loadbalancer

import (
	"context"
	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"sync"
	"testing"
)

type mockReverseProxy struct {
	called bool
	mu     sync.Mutex
}

func (m *mockReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.called = true
	m.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// LoadBalancerTestSuite is a test suite.
type LoadBalancerTestSuite struct {
	suite.Suite
}

// TestLoadBalancer runs the LoadBalancer test suite.
func TestLoadBalancer(t *testing.T) {
	suite.Run(t, new(LoadBalancerTestSuite))
}

// TestBackendNewSuccess verifies that NewBackend creates a backend with a valid URL.
func (s *LoadBalancerTestSuite) TestBackendNewSuccess() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)
	s.NotNil(backend)
	s.Equal("http://localhost:8080", backend.URL.String())
	s.True(backend.IsAlive())
	s.NotNil(backend.reverseProxy)
}

// TestBackendNewInvalidURL checks that NewBackend fails with an invalid URL.
func (s *LoadBalancerTestSuite) TestBackendNewInvalidURL() {
	backend, err := NewBackend("invalid_url")
	s.Error(err)
	s.Nil(backend)
}

// TestBackendAliveStatus tests IsAlive and SetAlive functionality.
func (s *LoadBalancerTestSuite) TestBackendAliveStatus() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)

	backend.maxFails = 1

	s.True(backend.IsAlive())
	backend.SetAlive(false)
	s.False(backend.IsAlive())
	backend.SetAlive(true)
	s.True(backend.IsAlive())
}

// TestBackendServeProxySuccess verifies that ServeProxy forwards requests correctly.
func (s *LoadBalancerTestSuite) TestBackendServeProxySuccess() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)
	mockProxy := &mockReverseProxy{}
	backend.reverseProxy = mockProxy

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	backend.ServeProxy(w, req)
	s.True(mockProxy.called)
	s.Equal(http.StatusOK, w.Code)
}

// TestBackendServeProxyCanceledContext checks that ServeProxy handle context cancellation.
func (s *LoadBalancerTestSuite) TestBackendServeProxyCanceledContext() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	backend.ServeProxy(w, req)
	s.Equal(http.StatusRequestTimeout, w.Code)
	s.Contains(w.Body.String(), "Request canceled")
}

// TestLeastConnBalancerNewInvalidConfig verifies that NewLeastConnBalancer panics with invalid configs.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerNewInvalidConfig() {
	assert.PanicsWithValue(s.T(), "at least one backend is required", func() {
		NewLeastConnBalancer()
	})

	backend, err := NewBackend("http://localhost:8080")
	s.Require().NoError(err)

	otherBackend, err := NewBackend("http://localhost:8081")
	s.Require().NoError(err)

	loads := map[*Backend]int{
		otherBackend: 0,
	}

	assert.PanicsWithValue(s.T(), "load count missing for backend", func() {
		NewLeastConnBalancer(
			WithBackends([]*Backend{backend}),
			WithInitialLoads(loads),
		)
	})
}

// TestLeastConnBalancerNewSuccess verifies that NewLeastConnBalancer initializes correctly.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerNewSuccess() {
	backend1, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend2, err := NewBackend("http://localhost:8082")
	s.NoError(err)

	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}))
	s.Len(lb.backends, 2)
	s.Equal(0, lb.loads[backend1])
	s.Equal(0, lb.loads[backend2])

	loads := map[*Backend]int{backend1: 5, backend2: 3}
	lb = NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}), WithInitialLoads(loads))
	s.Equal(5, lb.loads[backend1])
	s.Equal(3, lb.loads[backend2])
}

// TestLeastConnBalancerNextSuccess tests that Next select the backend with the least connections.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerNextSuccess() {
	backend1, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend2, err := NewBackend("http://localhost:8082")
	s.NoError(err)
	loads := map[*Backend]int{backend1: 2, backend2: 1}
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}), WithInitialLoads(loads))

	selected, err := lb.Next()
	s.NoError(err)
	s.Equal(backend2, selected)
	s.Equal(2, lb.loads[backend2])
}

// TestLeastConnBalancerNextNoAliveBackends checks that Next fails when no backends are alive.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerNextNoAliveBackends() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend.SetAlive(false)
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend}))

	_, err = lb.Next()
	s.Error(err)
	s.Contains(err.Error(), "no alive backends")
}

// TestLeastConnBalancerServeHTTPSuccess verifies that ServeHTTP forwards requests correctly.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerServeHTTPSuccess() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	mockProxy := &mockReverseProxy{}
	backend.reverseProxy = mockProxy
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	lb.ServeHTTP(w, req)
	s.True(mockProxy.called)
	s.Equal(http.StatusOK, w.Code)
	s.Equal(0, lb.loads[backend])
}

// TestLeastConnBalancerServeHTTPCanceledContext checks that ServeHTTP handles context cancellation.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerServeHTTPCanceledContext() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend}))

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	lb.ServeHTTP(w, req)
	s.Equal(http.StatusServiceUnavailable, w.Code)
	s.Contains(w.Body.String(), "context canceled")
}

// TestLeastConnBalancerRelease tests that Release decrements the load counter correctly.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerRelease() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend}))
	lb.loads[backend] = 2

	lb.Release(backend)
	s.Equal(1, lb.loads[backend])

	lb.Release(backend)
	s.Equal(0, lb.loads[backend])

	lb.Release(backend)
	s.Equal(0, lb.loads[backend]) // Should not go below zero
}

// TestLeastConnBalancerMarkFailure verifies that MarkFailure updates backend status and resets a load.
func (s *LoadBalancerTestSuite) TestLeastConnBalancerMarkFailure() {
	backend1, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend2, err := NewBackend("http://localhost:8082")
	s.NoError(err)

	// Set fail threshold to 1 so that single failure marks backend as dead
	backend1.maxFails = 1

	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}))
	lb.loads[backend1] = 5

	lb.MarkFailure(backend1)

	s.False(backend1.IsAlive())    // Will now be false
	s.Equal(0, lb.loads[backend1]) // Should be reset to 0

	selected, err := lb.Next()
	s.NoError(err)
	s.Equal(backend2, selected) // Should select only alive backend
}

// TestNewBalancerSuccess verifies that NewBalancer creates the correct balancer type.
func (s *LoadBalancerTestSuite) TestNewBalancerSuccess() {
	cfg := &config.BalancerConfig{
		Type:     LeastConnBalancerType,
		Backends: []string{"http://localhost:8081", "http://localhost:8082"},
	}
	// inject a no-op slog logger so signature matches NewBalancer(cfg, logger)
	logger := logging.NewLogger(&config.Config{LogType: "slog"})
	balancer, err := NewBalancer(cfg, logger)
	s.NoError(err)
	s.NotNil(balancer)
	s.IsType(&LeastConnBalancer{}, balancer)

	lb := balancer.(*LeastConnBalancer)
	s.Len(lb.backends, 2)
	s.Equal("http://localhost:8081", lb.backends[0].URL.String())
	s.Equal("http://localhost:8082", lb.backends[1].URL.String())
}

// TestNewBalancerInvalidType checks that NewBalancer fails with an unknown balancer type.
func (s *LoadBalancerTestSuite) TestNewBalancerInvalidType() {
	cfg := &config.BalancerConfig{
		Type:     "unknown",
		Backends: []string{"http://localhost:8081"},
	}
	logger := logging.NewLogger(&config.Config{LogType: "slog"})
	balancer, err := NewBalancer(cfg, logger)
	s.Error(err)
	s.Contains(err.Error(), "unknown balancer type")
	s.Nil(balancer)
}

// TestNewBalancerInvalidBackendURL verifies that NewBalancer fails with an invalid backend URL.
func (s *LoadBalancerTestSuite) TestNewBalancerInvalidBackendURL() {
	cfg := &config.BalancerConfig{
		Type:     LeastConnBalancerType,
		Backends: []string{"invalid_url"},
	}
	logger := logging.NewLogger(&config.Config{LogType: "slog"})
	balancer, err := NewBalancer(cfg, logger)
	s.Error(err)
	s.Contains(err.Error(), "failed to create backend")
	s.Nil(balancer)
}

// TestConcurrentNextOperations tests that Next handle concurrent calls correctly.
func (s *LoadBalancerTestSuite) TestConcurrentNextOperations() {
	backend1, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend2, err := NewBackend("http://localhost:8082")
	s.NoError(err)
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}))

	var wg sync.WaitGroup
	const numGoroutines = 10
	results := make([]*Backend, numGoroutines)
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			backend, err := lb.Next()
			s.NoError(err)
			results[idx] = backend
		}(i)
	}

	wg.Wait()

	// Verify load distribution
	backend1Count := 0
	backend2Count := 0
	for _, backend := range results {
		if backend == backend1 {
			backend1Count++
		} else if backend == backend2 {
			backend2Count++
		}
	}
	s.True(backend1Count > 0)
	s.True(backend2Count > 0)
	s.Equal(numGoroutines, backend1Count+backend2Count)
	s.Equal(numGoroutines, lb.loads[backend1]+lb.loads[backend2])

	// Release all loads
	for i := 0; i < numGoroutines; i++ {
		lb.Release(results[i])
	}
	s.Equal(0, lb.loads[backend1])
	s.Equal(0, lb.loads[backend2])
}

// TestConcurrentServeHTTPOperations tests that ServeHTTP handle concurrent requests correctly.
func (s *LoadBalancerTestSuite) TestConcurrentServeHTTPOperations() {
	backend1, err := NewBackend("http://localhost:8081")
	s.NoError(err)
	backend2, err := NewBackend("http://localhost:8082")
	s.NoError(err)
	mockProxy1 := &mockReverseProxy{}
	mockProxy2 := &mockReverseProxy{}
	backend1.reverseProxy = mockProxy1
	backend2.reverseProxy = mockProxy2
	lb := NewLeastConnBalancer(WithBackends([]*Backend{backend1, backend2}))

	var wg sync.WaitGroup
	const numRequests = 100
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			lb.ServeHTTP(w, req)
			s.Equal(http.StatusOK, w.Code)
		}()
	}

	wg.Wait()
	s.True(mockProxy1.called)
	s.True(mockProxy2.called)
	s.Equal(0, lb.loads[backend1])
	s.Equal(0, lb.loads[backend2])
}

// TestErrorHandler verifies that the reverse proxy's ErrorHandler handles errors correctly.
func (s *LoadBalancerTestSuite) TestErrorHandler() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)

	// Create a real reverse proxy and configure its ErrorHandler
	proxy := httputil.NewSingleHostReverseProxy(backend.URL)
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		if req.Context().Err() != nil {
			http.Error(w, "Request canceled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
	}

	// Assign the configured proxy to the http.Handler field
	backend.reverseProxy = proxy

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	backend.ServeProxy(w, req)
	s.Equal(http.StatusRequestTimeout, w.Code)
	s.Contains(w.Body.String(), "Request canceled")
}
