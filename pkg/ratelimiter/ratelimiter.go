package ratelimiter

import (
	"context"
	"errors"
	"github.com/YuarenArt/GoBalancer/internal/config"
)

var (
	// ErrNoSuchClient is returned when a client is not found in the configuration.
	ErrNoSuchClient = errors.New("client not found")

	tokenBucketType = "token_bucket"
)

// RateLimiter defines the interface for rate limiting algorithms.
type RateLimiter interface {
	// Allow checks if a request can be processed based on the current rate limit.
	Allow(ctx context.Context, clientID string) bool
	// SetClientConfig sets the custom rate limiting configuration for a specific client.
	SetClientConfig(ctx context.Context, clientID string, cfg ClientConfig) error
	// Stop halts the refilling process and stops the RateLimiter.
	Stop() error
	// ClientConfig returns the configuration for a client.
	ClientConfig(ctx context.Context, clientID string) (ClientConfig, error)
}

// NewRateLimiter creates a RateLimiter instance based on the configuration.
func NewRateLimiter(ctx context.Context, cfg *config.RateLimitConfig) (RateLimiter, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	switch cfg.Type {
	case tokenBucketType:
		return NewBucketRateLimiter(
			ctx,
			WithCapacity(cfg.DefaultBurst),
			WithRate(cfg.DefaultRate),
			WithTickerDuration(cfg.TickerDuration),
		)
	default:
		return nil, errors.New("unknown limiter type: " + cfg.Type)
	}
}
