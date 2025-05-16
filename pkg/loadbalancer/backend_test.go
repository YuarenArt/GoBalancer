package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/stretchr/testify/suite"
)

// mockReverseProxy is a fake reverse proxy used to test proxying logic.
type mockReverseProxy struct {
	called bool
}

// ServeHTTP simulates proxy response with 200 OK.
func (m *mockReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.called = true
	w.WriteHeader(http.StatusOK)
}

// BackendTest includes unit tests for the Backend struct.
type BackendTest struct {
	suite.Suite
}

func TestBackend(t *testing.T) {
	suite.Run(t, new(BackendTest))
}

// TestBackendNewSuccess verifies Backend is created with valid URL.
func (s *BackendTest) TestBackendNewSuccess() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)
	s.NotNil(backend)
	s.Equal("http://localhost:8080", backend.URL.String())
	s.True(backend.IsAlive())
	s.NotNil(backend.reverseProxy)
}

// TestBackendNewInvalidURL ensures invalid URL returns an error.
func (s *BackendTest) TestBackendNewInvalidURL() {
	backend, err := NewBackend("invalid_url")
	s.Error(err)
	s.Nil(backend)
}

// TestBackendAliveStatus checks Get/SetAlive behavior.
func (s *BackendTest) TestBackendAliveStatus() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)
	backend.maxFails = 1
	s.True(backend.IsAlive())

	backend.SetAlive(false)
	s.False(backend.IsAlive())

	backend.SetAlive(true)
	s.True(backend.IsAlive())
}

// TestBackendServeProxySuccess validates proxying through reverseProxy.
func (s *BackendTest) TestBackendServeProxySuccess() {
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

// TestBackendServeProxyCanceledContext verifies timeout behavior.
func (s *BackendTest) TestBackendServeProxyCanceledContext() {
	backend, err := NewBackend("http://localhost:8080")
	s.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	backend.ServeProxy(w, req)
	s.Equal(http.StatusRequestTimeout, w.Code)
	s.Contains(w.Body.String(), "Request canceled")
}

// TestErrorHandler verifies custom error handler for reverse proxy.
func (s *BackendTest) TestErrorHandler() {
	backend, err := NewBackend("http://localhost:8081")
	s.NoError(err)

	proxy := httputil.NewSingleHostReverseProxy(backend.URL)
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		if req.Context().Err() != nil {
			http.Error(w, "Request canceled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
	}
	backend.reverseProxy = proxy

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	backend.ServeProxy(w, req)
	s.Equal(http.StatusRequestTimeout, w.Code)
	s.Contains(w.Body.String(), "Request canceled")
}
