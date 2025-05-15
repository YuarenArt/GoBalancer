package ratelimiter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/YuarenArt/GoBalancer/internal/config"
	"github.com/YuarenArt/GoBalancer/pkg/ratelimiter"
)

// RateLimiterTestSuite defines the test suite for rate limiter tests.
type RateLimiterTestSuite struct {
	suite.Suite
	rl   *ratelimiter.BucketRateLimiter
	rlim ratelimiter.RateLimiter
	ctx  context.Context
}

// SetupSuite initializes shared resources.
func (s *RateLimiterTestSuite) SetupSuite() {
	s.ctx = context.Background()
}

// TearDownSuite cleans up resources.
func (s *RateLimiterTestSuite) TearDownSuite() {
	if s.rl != nil {
		s.rl.Stop()
	}
	if s.rlim != nil {
		s.rlim.Stop()
	}
}

// TestBucket_AllowAndRefill tests basic Bucket functionality.
func (s *RateLimiterTestSuite) TestBucket_AllowAndRefill() {
	b := ratelimiter.NewBucket(2, 1)

	s.True(b.Allow(), "should allow when tokens available")
	s.True(b.Allow(), "should allow second token")
	s.False(b.Allow(), "should not allow when no tokens")

	b.Refill()
	s.True(b.Allow(), "should allow after refill")
	s.False(b.Allow(), "should not allow when one refill token consumed and empty")
}

// TestNewBucketRateLimiter_InvalidConfig tests creation with invalid configurations.
func (s *RateLimiterTestSuite) TestNewBucketRateLimiter_InvalidConfig() {
	_, err := ratelimiter.NewBucketRateLimiter(s.ctx, ratelimiter.WithCapacity(0))
	s.Error(err, "should return error for zero capacity")

	_, err = ratelimiter.NewBucketRateLimiter(s.ctx, ratelimiter.WithRate(0))
	s.Error(err, "should return error for zero rate")

	rl, err := ratelimiter.NewBucketRateLimiter(s.ctx)
	s.Require().NoError(err)
	rl.Stop() // Verify that stopping works
}

// TestBucketRateLimiter_Allow_NewBucket tests Allow for a new bucket.
func (s *RateLimiterTestSuite) TestBucketRateLimiter_Allow_NewBucket() {
	tickerDuration := 100 * time.Millisecond
	rl, err := ratelimiter.NewBucketRateLimiter(s.ctx,
		ratelimiter.WithCapacity(3),
		ratelimiter.WithRate(1),
		ratelimiter.WithTickerDuration(tickerDuration),
	)
	s.Require().NoError(err)
	s.rl = rl

	client := "client1"
	for i := 0; i < 3; i++ {
		s.True(rl.Allow(s.ctx, client), "should allow until capacity is reached")
	}
	s.False(rl.Allow(s.ctx, client), "should not allow after capacity is exhausted")

	time.Sleep(tickerDuration + 50*time.Millisecond) // Buffer for reliability
	s.True(rl.Allow(s.ctx, client), "should allow after refill")
}

// TestBucketRateLimiter_SetAndGetConfig tests setting and retrieving client configurations.
func (s *RateLimiterTestSuite) TestBucketRateLimiter_SetAndGetConfig() {
	rl, err := ratelimiter.NewBucketRateLimiter(s.ctx,
		ratelimiter.WithCapacity(5),
		ratelimiter.WithRate(2),
		ratelimiter.WithTickerDuration(50*time.Millisecond),
	)
	s.Require().NoError(err)
	s.rl = rl

	client := "clientA"
	cfg := ratelimiter.ClientConfig{Capacity: 2, RefillRate: 2}
	err = rl.SetClientConfig(s.ctx, client, cfg)
	s.Require().NoError(err)

	got, err := rl.ClientConfig(s.ctx, client)
	s.Require().NoError(err)
	s.Equal(cfg, got)

	_, err = rl.ClientConfig(s.ctx, "noSuch")
	s.ErrorIs(err, ratelimiter.ErrNoSuchClient)

	// Test updating configuration
	newCfg := ratelimiter.ClientConfig{Capacity: 3, RefillRate: 3}
	err = rl.SetClientConfig(s.ctx, client, newCfg)
	s.Require().NoError(err)
	got, err = rl.ClientConfig(s.ctx, client)
	s.Require().NoError(err)
	s.Equal(newCfg, got)
}

// TestAllow_ContextCanceled tests behavior with a canceled context.
func (s *RateLimiterTestSuite) TestAllow_ContextCanceled() {
	rctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl, err := ratelimiter.NewBucketRateLimiter(rctx,
		ratelimiter.WithCapacity(1),
		ratelimiter.WithRate(1),
	)
	s.Require().NoError(err)
	s.rl = rl

	s.False(rl.Allow(rctx, "x"), "should return false when context is canceled")
}

// TestStop_Idempotent tests idempotency of the Stop method.
func (s *RateLimiterTestSuite) TestStop_Idempotent() {
	rl, err := ratelimiter.NewBucketRateLimiter(s.ctx)
	s.Require().NoError(err)

	err = rl.Stop()
	s.NoError(err)
	err = rl.Stop()
	s.Error(err, "should return error on repeated stop")
}

// TestNewRateLimiter_Function tests the NewRateLimiter factory function.
func (s *RateLimiterTestSuite) TestNewRateLimiter_Function() {
	rlim, err := ratelimiter.NewRateLimiter(s.ctx, &config.RateLimitConfig{Enabled: false})
	s.Require().NoError(err)
	s.Nil(rlim)

	rlim, err = ratelimiter.NewRateLimiter(s.ctx, &config.RateLimitConfig{Enabled: true, Type: "unknown"})
	s.Error(err, "should return error for unknown limiter type")

	rlim, err = ratelimiter.NewRateLimiter(s.ctx, &config.RateLimitConfig{
		Enabled:        true,
		Type:           "token_bucket",
		DefaultBurst:   5,
		DefaultRate:    1,
		TickerDuration: time.Millisecond,
	})
	s.Require().NoError(err)
	s.NotNil(rlim)
	_, ok := rlim.(ratelimiter.RateLimiter)
	s.True(ok)

	// Verify stopping works
	err = rlim.Stop()
	s.NoError(err)
}

func TestRateLimiterTestSuite(t *testing.T) {
	suite.Run(t, new(RateLimiterTestSuite))
}
