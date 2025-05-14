package config

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds settings for the application.
type Config struct {
	HTTPPort  string          // Port for HTTP server (e.g., "8080")
	Balancer  BalancerConfig  // Load balancer settings
	RateLimit RateLimitConfig // Rate limiting settings
}

// RateLimitConfig holds settings for the rate limiter (Token Bucket).
type RateLimitConfig struct {
	Enabled        bool          // Enable rate limiting
	DefaultRate    int           // Tokens per second
	DefaultBurst   int           // Bucket capacity
	Type           string        // Rate limiter algorithm type (e.g., "token_bucket")
	TickerDuration time.Duration // Duration for ticker interval (e.g., 1s)
}

// BalancerConfig holds settings for the load balancer.
type BalancerConfig struct {
	Type     string   // Balancer algorithm type (e.g., "least_conn", "round_robin")
	Backends []string // List of backend addresses
}

// NewConfig initializes configuration with priority:
// 1. Command-line flags
// 2. Environment variables
// 3. Config file (YAML or JSON)
// 4. Defaults
func NewConfig() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Load general flags
	httpPort := flag.String("port", v.GetString("http_port"), "HTTP server port")
	flag.Parse()

	rateLimitCfg, err := loadRateLimitConfig(v)
	if err != nil {
		return nil, fmt.Errorf("error loading rate limit config: %w", err)
	}

	balancerCfg, err := loadBalancerConfig(v)
	if err != nil {
		return nil, fmt.Errorf("error loading balancer config: %w", err)
	}

	return &Config{
		HTTPPort:  resolve(v, "http_port", *httpPort),
		RateLimit: rateLimitCfg,
		Balancer:  balancerCfg,
	}, nil
}

// loadRateLimitConfig loads configuration for the rate limiter.
func loadRateLimitConfig(v *viper.Viper) (RateLimitConfig, error) {
	rateLimit := flag.Bool("rate-limit", v.GetBool("rate_limit.enabled"), "Enable rate limiting")
	rate := flag.Int("rate-limit-rate", v.GetInt("rate_limit.default_rate"), "Tokens per second")
	burst := flag.Int("rate-limit-burst", v.GetInt("rate_limit.default_burst"), "Bucket capacity")
	limiterType := flag.String("rate-limit-type", v.GetString("rate_limit.type"), "Rate limiter algorithm type (e.g., token-bucket)")
	tickerDuration := flag.Duration("rate-limit-ticker", v.GetDuration("rate_limit.ticker_duration"), "Ticker interval duration")
	flag.Parse()

	return RateLimitConfig{
		Enabled:        resolve(v, "rate_limit.enabled", *rateLimit),
		DefaultRate:    resolve(v, "rate_limit.default_rate", *rate),
		DefaultBurst:   resolve(v, "rate_limit.default_burst", *burst),
		Type:           resolve(v, "rate_limit.type", *limiterType),
		TickerDuration: resolve(v, "rate_limit.ticker_duration", *tickerDuration),
	}, nil
}

// loadBalancerConfig loads configuration for the load balancer.
func loadBalancerConfig(v *viper.Viper) (BalancerConfig, error) {
	balancerType := flag.String("balancer-type", v.GetString("balancer.type"), "Load balancer algorithm type (e.g., least_conn, round_robin)")
	flag.Parse()

	return BalancerConfig{
		Type:     resolve(v, "balancer.type", *balancerType),
		Backends: v.GetStringSlice("backends"),
	}, nil
}

// resolve chooses the flag value if it differs from viper's, otherwise uses viper's.
func resolve[T comparable](v *viper.Viper, key string, flagVal T) T {
	viperVal := v.Get(key)
	if val, ok := viperVal.(T); ok && flagVal != val {
		return flagVal
	}
	return v.Get(key).(T)
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	v.SetDefault("http_port", "8080")
	v.SetDefault("backends", []string{"http://localhost:8081", "http://localhost:8082"})
	v.SetDefault("log_level", "info")
	v.SetDefault("log_to_file", false)
	v.SetDefault("log_file_path", "logs/balancer.log")
	v.SetDefault("rate_limit.enabled", false)
	v.SetDefault("rate_limit.default_rate", 10.0)
	v.SetDefault("rate_limit.default_burst", 20)
	v.SetDefault("rate_limit.type", "token_bucket")
	v.SetDefault("rate_limit.ticker_duration", time.Second)
	v.SetDefault("balancer.type", "least_conn")
}
