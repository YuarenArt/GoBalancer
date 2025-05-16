package loadbalancer

import (
	"context"
	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
	"net"
	"net/http"
	"sync"
	"time"
)

// HealthChecker periodically checks the health status of all backends
type HealthChecker struct {
	client   *http.Client   // HTTP client for health checks
	backends []*Backend     // List of backends to check
	logger   logging.Logger // Logger for health check events

	interval time.Duration // Interval between health check cycles
	timeout  time.Duration // Timeout for individual health check requests
	path     string        // Path for health check requests

	workerPool chan struct{}      // Limits concurrent health checks
	cancel     context.CancelFunc // Cancels the health check context
	wg         sync.WaitGroup     // Tracks running goroutines
	stopOnce   sync.Once          // Ensures Stop is called only once
}

func WithHealthLogger(logger logging.Logger) HealthOption {
	return func(hc *HealthChecker) {
		hc.logger = logger
	}
}

// HealthOption configures a HealthChecker.
type HealthOption func(hc *HealthChecker)

// WithHealthInterval sets the health check interval.
func WithHealthInterval(d time.Duration) HealthOption {
	return func(hc *HealthChecker) { hc.interval = d }
}

// WithHealthTimeout sets the timeout per single health check request.
func WithHealthTimeout(d time.Duration) HealthOption {
	return func(hc *HealthChecker) { hc.timeout = d }
}

// WithHealthPath sets the HTTP path to use when checking health status.
func WithHealthPath(p string) HealthOption {
	return func(hc *HealthChecker) { hc.path = p }
}

// WithHealthConcurrency sets the number of concurrent health check workers.
func WithHealthConcurrency(n int) HealthOption {
	return func(hc *HealthChecker) {
		if n <= 0 {
			n = 10
		}
		hc.workerPool = make(chan struct{}, n)
	}
}

// NewHealthChecker creates a new HealthChecker with provided options.
func NewHealthChecker(backends []*Backend, logger logging.Logger, opts ...HealthOption) *HealthChecker {
	hc := &HealthChecker{
		backends: backends,
		interval: 10 * time.Second,
		timeout:  2 * time.Second,
		logger:   logger,
		path:     "/health",
		client: &http.Client{
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 2 * time.Second,
			},
			Timeout: 2 * time.Second,
		},
		workerPool: make(chan struct{}, 10),
	}
	for _, opt := range opts {
		opt(hc)
	}

	// Update client timeout to match configured timeout
	hc.client.Timeout = hc.timeout
	if t, ok := hc.client.Transport.(*http.Transport); ok {
		t.DialContext = (&net.Dialer{Timeout: hc.timeout}).DialContext
		t.TLSHandshakeTimeout = hc.timeout
	}

	if hc.logger == nil {
		hc.logger = logging.NewLogger(&config.Config{LogType: "slog"})
	}

	hc.checkAll(context.Background())
	return hc
}

// Run starts the periodic health checking process.
func (hc *HealthChecker) Run(ctx context.Context) {
	var cctx context.Context
	cctx, hc.cancel = context.WithCancel(ctx)

	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		ticker := time.NewTicker(hc.interval)
		defer ticker.Stop()

		hc.logger.Info("starting health checker",
			"interval", hc.interval,
			"timeout", hc.timeout,
			"path", hc.path,
		)

		for {
			select {
			case <-ticker.C:
				hc.checkAll(cctx)
			case <-cctx.Done():
				hc.logger.Info("stopping health checker")
				return
			}
		}
	}()
}

// Stop signals the health checker to stop and waits for the background goroutine to finish.
// Also closes the underlying HTTP transport connections.
func (hc *HealthChecker) Stop() {
	hc.stopOnce.Do(func() {
		if hc.cancel != nil {
			hc.cancel()
		}
		hc.wg.Wait()
		if t, ok := hc.client.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
	})
}

// checkAll launches concurrent health checks for all backends using a worker pool.
func (hc *HealthChecker) checkAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, b := range hc.backends {
		select {
		case hc.workerPool <- struct{}{}: // Acquire worker slot
			wg.Add(1)
			go func(b *Backend) {
				defer wg.Done()
				defer func() { <-hc.workerPool }() // Release worker slot
				hc.checkOne(ctx, b)
			}(b)
		case <-ctx.Done():
			hc.logger.Warn("Health check cycle interrupted", "error", ctx.Err())
			return
		}
	}
	wg.Wait()
	hc.logger.Debug("Health check cycle completed", "backends_checked", len(hc.backends))
}

// checkOne performs a single HTTP health check on a given backend.
func (hc *HealthChecker) checkOne(ctx context.Context, b *Backend) {
	u := *b.URL // Copy to avoid modifying original
	u.Path = hc.path

	reqCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		hc.logger.Error("Failed to create health check request",
			"url", u.String(),
			"error", err,
		)
		b.SetAlive(false)
		return
	}

	// Add headers to mimic a real request
	req.Header.Set("User-Agent", "GoBalancer-HealthCheck/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.client.Do(req)
	if err != nil {
		// Check if the error is due to context cancellation
		if reqCtx.Err() != nil {
			hc.logger.Warn("Health check canceled",
				"url", u.String(),
				"error", reqCtx.Err(),
			)
		} else {
			hc.logger.Error("Health check failed",
				"url", u.String(),
				"error", err,
			)
		}
		b.SetAlive(false)
		return
	}
	defer resp.Body.Close()

	// Consider 2xx status codes as healthy
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		b.SetAlive(true)
		hc.logger.Info("Backend is healthy",
			"url", u.String(),
			"status", resp.StatusCode,
		)
	} else {
		hc.logger.Warn("Backend returned unhealthy status",
			"url", u.String(),
			"status", resp.StatusCode,
		)
		b.SetAlive(false)
	}
}
