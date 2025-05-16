package loadbalancer

import (
	"context"
	"fmt"
	"time"

	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
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
	// StartHealthChecks starts health checking for backends.
	StartHealthChecks(ctx context.Context) error
	// StopHealthChecks stops the health checker.
	StopHealthChecks()
}

// NewBalancer creates Balancer instances based on the configuration.
func NewBalancer(cfg *config.BalancerConfig, logger logging.Logger) (Balancer, error) {
	backends := make([]*Backend, 0, len(cfg.Backends))
	for _, addr := range cfg.Backends {
		backend, err := NewBackend(addr)
		if err != nil {
			logger.Error("Failed to create backend", "address", addr, "error", err)
			return nil, fmt.Errorf("failed to create backend: %w", err)
		}
		backends = append(backends, backend)
	}

	healthChecker := NewHealthChecker(
		backends,
		logger,
		WithHealthInterval(10*time.Second),
		WithHealthTimeout(10*time.Second),
		WithHealthPath("/health"),
		WithHealthConcurrency(10),
	)

	switch cfg.Type {
	case LeastConnBalancerType:
		lb := NewLeastConnBalancer(
			WithBackends(backends),
			WithHealthChecker(healthChecker),
		)
		if err := lb.StartHealthChecks(context.Background()); err != nil {
			logger.Error("Failed to start health checks", "error", err)
			return nil, fmt.Errorf("failed to start health checks: %w", err)
		}
		return lb, nil
	default:
		logger.Error("Unknown balancer type", "type", cfg.Type)
		return nil, fmt.Errorf("unknown balancer type: %s", cfg.Type)
	}
}
