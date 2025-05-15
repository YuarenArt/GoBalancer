package ratelimiter

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Bucket stores the state of the Token Bucket for a single client.
type Bucket struct {
	capacity   int // maximum number of tokens
	tokens     int // current number of tokens
	refillRate int // tokens to refill per interval
	mutex      sync.Mutex
}

// ClientConfig holds the rate limiting configuration for a client.
type ClientConfig struct {
	Capacity   int
	RefillRate int
}

// NewBucket creates a new Bucket with specified capacity and refill rate.
func NewBucket(capacity, refillRate int) *Bucket {
	return &Bucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
	}
}

// Allow checks if a token is available and consumes one token if possible.
func (b *Bucket) Allow() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

// Refill adds tokens to the bucket according to the refill rate and respects the capacity.
func (b *Bucket) Refill() {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.tokens += b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}

// Option defines a function that applies a configuration to the limiter.
type Option func(*BucketRateLimiter)

// WithCapacity overrides the default capacity.
func WithCapacity(cap int) Option {
	return func(rl *BucketRateLimiter) {
		rl.defaultCap = cap
	}
}

// WithRate overrides the default refill rate.
func WithRate(rate int) Option {
	return func(rl *BucketRateLimiter) {
		rl.defaultRate = rate
	}
}

// WithTickerDuration overrides the ticker interval for refills.
func WithTickerDuration(d time.Duration) Option {
	return func(rl *BucketRateLimiter) {
		rl.ticker = time.NewTicker(d)
	}
}

// WithBuckets sets the initial buckets for the rate limiter.
func WithBuckets(buckets map[string]*Bucket) Option {
	return func(rl *BucketRateLimiter) {
		rl.buckets = buckets
	}
}

// BucketRateLimiter manages multiple buckets and their refilling.
type BucketRateLimiter struct {
	buckets     map[string]*Bucket
	defaultCap  int
	defaultRate int
	mutex       sync.RWMutex
	ticker      *time.Ticker
	stopChan    chan struct{}
	closed      bool
	closedMutex sync.Mutex
}

// NewBucketRateLimiter creates a new BucketRateLimiter with the specified options and context.
// The provided context controls the lifecycle of the refill goroutine.
func NewBucketRateLimiter(ctx context.Context, options ...Option) (*BucketRateLimiter, error) {
	// Initialize with default values
	rl := &BucketRateLimiter{
		buckets:     make(map[string]*Bucket),
		defaultCap:  20,                          // Default capacity
		defaultRate: 10,                          // Default rate
		ticker:      time.NewTicker(time.Second), // Default ticker interval
		stopChan:    make(chan struct{}),
	}

	// Apply provided options
	for _, opt := range options {
		opt(rl)
	}

	// Validate configuration
	if err := rl.validate(); err != nil {
		return nil, err
	}

	// Start the refill goroutine with the provided context
	go rl.startRefill(ctx)
	return rl, nil
}

// startRefill runs a goroutine that refills all buckets periodically based on ticker.
// It exits when the context is canceled or stopChan is closed.
func (rl *BucketRateLimiter) startRefill(ctx context.Context) {
	for {
		select {
		case <-rl.ticker.C:
			rl.refillAll()
		case <-rl.stopChan:
			rl.ticker.Stop()
			return
		case <-ctx.Done():
			rl.ticker.Stop()
			return
		}
	}
}

// refillAll refills all the buckets by calling refill on each.
func (rl *BucketRateLimiter) refillAll() {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	for _, b := range rl.buckets {
		b.Refill()
	}
}

// SetClientConfig saves custom rate limiting configuration for a client.
// If the client already has a bucket, it will be recreated with the new configuration.
// Respects the provided context for cancellation.
func (rl *BucketRateLimiter) SetClientConfig(ctx context.Context, clientID string, cfg ClientConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.buckets[clientID] = NewBucket(cfg.Capacity, cfg.RefillRate)
	return nil
}

// Allow checks if a request from the client can be processed based on their token bucket.
// If no bucket exists, it creates a new one with default parameters.
// Returns false if the context is canceled or timed out.
func (rl *BucketRateLimiter) Allow(ctx context.Context, clientID string) bool {
	// Check if the context is canceled or timed out
	if err := ctx.Err(); err != nil {
		return false
	}

	rl.mutex.RLock()
	b, exists := rl.buckets[clientID]
	rl.mutex.RUnlock()

	if !exists {
		rl.mutex.Lock()
		b = NewBucket(rl.defaultCap, rl.defaultRate)
		rl.buckets[clientID] = b
		rl.mutex.Unlock()
	}

	return b.Allow()
}

// Stop stops the ticker and the refill goroutine with a timeout.
// It ensures the limiter is not used after stopping.
func (rl *BucketRateLimiter) Stop() error {
	rl.closedMutex.Lock()
	if rl.closed {
		rl.closedMutex.Unlock()
		return errors.New("rate limiter already stopped")
	}
	rl.closed = true
	rl.closedMutex.Unlock()

	// Create a context with a timeout to ensure graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Signal the refill goroutine to stop
	close(rl.stopChan)

	// Wait for the ticker to stop or timeout
	select {
	case <-ctx.Done():
		return errors.New("timeout waiting for rate limiter to stop")
	case <-time.After(100 * time.Millisecond): // Give a small buffer for the goroutine to exit
		return nil
	}
}

// ClientConfig returns the current configuration of the bucket for the client.
// Respects the provided context for cancellation.
func (rl *BucketRateLimiter) ClientConfig(ctx context.Context, clientID string) (ClientConfig, error) {
	if err := ctx.Err(); err != nil {
		return ClientConfig{}, err
	}

	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	b, exists := rl.buckets[clientID]
	if !exists {
		return ClientConfig{}, ErrNoSuchClient
	}
	// Read the bucket parameters atomically
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return ClientConfig{
		Capacity:   b.capacity,
		RefillRate: b.refillRate,
	}, nil
}

// validate ensures the BucketRateLimiter configuration is valid.
func (rl *BucketRateLimiter) validate() error {
	if rl.defaultCap <= 0 {
		return errors.New("default capacity must be positive")
	}
	if rl.defaultRate <= 0 {
		return errors.New("default rate must be positive")
	}
	if rl.ticker == nil {
		return errors.New("ticker must not be nil")
	}
	return nil
}
