package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/internal/logging"
	"github.com/YuarenArt/GoBalancer/pkg/loadbalancer"
	"github.com/YuarenArt/GoBalancer/pkg/ratelimiter"
	"net"
	"net/http"
	"time"
)

// ErrorResponse defines the structure for JSON error responses.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// wrappedWriter captures status code for logging
type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Server encapsulates the HTTP server, load balancer, and rate limiter.
type Server struct {
	balancer    loadbalancer.Balancer
	rateLimiter ratelimiter.RateLimiter
	httpServer  *http.Server
	logger      logging.Logger
}

// NewServer initializes a new Server with given configuration.
func NewServer(cfg *config.Config, logger logging.Logger) (*Server, error) {
	bal, err := loadbalancer.NewBalancer(&cfg.Balancer)
	if err != nil {
		logger.Error("Failed to initialize balancer", "error", err)
		return nil, fmt.Errorf("failed to initialize balancer: %w", err)
	}

	// Create rate limiter (may be nil if disabled)
	rl, err := ratelimiter.NewRateLimiter(context.Background(), &cfg.RateLimit)
	if err != nil {
		logger.Error("Failed to initialize rate limiter", "error", err)
		return nil, fmt.Errorf("failed to initialize rate limiter: %w", err)
	}

	// Compose middlewares: logging → rate limit → balancer
	handler := loggingMiddleware(logger, rateLimitMiddleware(logger, rl, bal.(http.Handler)))

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	return &Server{
		balancer:    bal,
		rateLimiter: rl,
		httpServer:  srv,
		logger:      logger,
	}, nil
}

// Start launches the HTTP server and listens for incoming connections.
func (s *Server) start() <-chan error {
	errChan := make(chan error, 1)
	s.logger.Info("Starting HTTP server", "addr", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server error", "error", err)
			errChan <- fmt.Errorf("HTTP server failed: %w", err)
		}
		close(errChan)
	}()

	return errChan
}

// Run starts the HTTP server and gracefully shuts it down upon context cancellation.
func (s *Server) Run(ctx context.Context) error {
	errChan := s.start()

	select {
	case <-ctx.Done():
		s.logger.Info("context canceled, initiating shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("graceful shutdown failed", "error", err)
			return err
		}
		return ctx.Err()

	case err := <-errChan:
		if err != nil {
			s.logger.Error("server error occurred", "error", err)
			return err
		}
		s.logger.Info("server exited cleanly")
		return nil
	}
}

// Shutdown gracefully stops the server handling in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server", "addr", s.httpServer.Addr)

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Server shutdown failed", "addr", s.httpServer.Addr, "error", err)
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	if s.rateLimiter != nil {
		if err := s.rateLimiter.Stop(); err != nil {
			s.logger.Error("Rate limiter shutdown failed", "error", err)
			return fmt.Errorf("rate limiter shutdown failed: %w", err)
		}
	}

	s.logger.Info("Server shutdown completed", "addr", s.httpServer.Addr)
	return nil
}

// loggingMiddleware logs the method, path, duration, and status code of the request.
func loggingMiddleware(logger logging.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			logger.Error("Failed to parse client IP", "error", err, "remote_addr", r.RemoteAddr)
			http.Error(w, `{"code": 400, "message": "Invalid client address"}`, http.StatusBadRequest)
			return
		}

		logger.Info("Received request",
			"method", r.Method,
			"path", r.URL.Path,
			"client_ip", clientIP,
		)
		logger.Debug("Request headers",
			"client_ip", clientIP,
			"headers", r.Header,
		)

		// Check if request is canceled
		if err := r.Context().Err(); err != nil {
			logger.Warn("Request canceled",
				"client_ip", clientIP,
				"path", r.URL.Path,
				"error", err,
			)
			http.Error(w, `{"code": 408, "message": "Request canceled"}`, http.StatusRequestTimeout)
			return
		}

		// Wrap response writer to capture status code
		ww := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)

		duration := time.Since(start)

		logger.Info("Request served",
			"method", r.Method,
			"path", r.URL.Path,
			"client_ip", clientIP,
			"status", ww.statusCode,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// rateLimitMiddleware wraps handler with token bucket rate limiting.
func rateLimitMiddleware(logger logging.Logger, rl ratelimiter.RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			logger.Error("Failed to parse client IP", "error", err, "remote_addr", r.RemoteAddr)
			http.Error(w, `{"code": 400, "message": "Invalid client address"}`, http.StatusBadRequest)
			return
		}

		if !rl.Allow(r.Context(), clientIP) {
			logger.Warn("Rate limit exceeded", "client_ip", clientIP)
			http.Error(w, `{"code": 429, "message": "Rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		logger.Debug("Rate limit check passed", "client_ip", clientIP)
		next.ServeHTTP(w, r)
	})
}
