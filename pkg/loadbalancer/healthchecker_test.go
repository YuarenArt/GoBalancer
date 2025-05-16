package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
)

type HealthCheckerTestSuite struct {
	suite.Suite
	logger logging.Logger
}

func (s *HealthCheckerTestSuite) SetupTest() {
	// Initialize a fresh logger for each test to avoid log pollution
	cfg := &config.Config{LogType: "slog"}
	s.logger = logging.NewLogger(cfg)
}

func TestHealthCheckerSuite(t *testing.T) {
	suite.Run(t, new(HealthCheckerTestSuite))
}

func (s *HealthCheckerTestSuite) TestHealthyEndpointMarksAlive() {
	// server always returns 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	backend, err := NewBackend(srv.URL)
	s.Require().NoError(err)
	logger := logging.NewLogger(&config.Config{LogType: "slog"})

	// very short timeout so checkAll runs promptly
	hc := NewHealthChecker(
		[]*Backend{backend},
		logger,
		WithHealthInterval(50*time.Millisecond),
		WithHealthTimeout(100*time.Millisecond),
		WithHealthPath("/"), // root path
		WithHealthConcurrency(1),
	)
	// run one cycle manually
	hc.checkAll(context.Background())

	s.True(backend.IsAlive(), "backend should be marked alive on 200")
}

func (s *HealthCheckerTestSuite) TestUnhealthyEndpointMarksDown() {
	// Server returns 500 Internal Server Error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	backend, err := NewBackend(srv.URL)
	s.Require().NoError(err)
	backend.maxFails = 1
	backend.SetAlive(true) // Start with backend alive to test transition

	hc := NewHealthChecker(
		[]*Backend{backend},
		s.logger,
		WithHealthInterval(50*time.Millisecond),
		WithHealthTimeout(100*time.Millisecond),
		WithHealthPath("/"),
		WithHealthConcurrency(1),
	)

	// Run one health check cycle
	hc.checkAll(context.Background())

	s.False(backend.IsAlive(), "backend should be marked down on 500")
}

func (s *HealthCheckerTestSuite) TestTimeoutMarksDown() {
	// server delays longer than timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	backend, err := NewBackend(srv.URL)
	s.Require().NoError(err)
	backend.maxFails = 1
	logger := logging.NewLogger(&config.Config{LogType: "slog"})

	hc := NewHealthChecker(
		[]*Backend{backend},
		logger,
		WithHealthInterval(50*time.Millisecond),
		WithHealthTimeout(50*time.Millisecond),
		WithHealthPath("/"),
		WithHealthConcurrency(1),
	)
	hc.checkAll(context.Background())

	s.False(backend.IsAlive(), "backend should be marked down on timeout")
}

func (s *HealthCheckerTestSuite) TestRunAndStopCycle() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	backend, err := NewBackend(srv.URL)
	s.Require().NoError(err)
	logger := logging.NewLogger(&config.Config{LogType: "slog"})

	hc := NewHealthChecker([]*Backend{backend}, logger,
		WithHealthInterval(10*time.Millisecond),
		WithHealthTimeout(20*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	hc.Stop()

	s.True(backend.IsAlive())
}
