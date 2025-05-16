package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds settings for the application.
type Config struct {
	HTTPPort  string          // Port for HTTP server (e.g., "8080")
	Balancer  BalancerConfig  // Load balancer settings
	RateLimit RateLimitConfig // Rate limiting settings
	LogType   string          // Logger type (e.g., "slog")
	LogToFile bool            // Enable logging to file
	LogFile   string          // Path to log file (e.g., "logs/app.log")
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

	// Handle ENV override for backends from comma-separated string
	if val := os.Getenv("BACKENDS"); val != "" {
		v.Set("backends", strings.Split(val, ","))
	}

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Load general flags as strings
	httpPort := flag.String("port", v.GetString("http_port"), "HTTP server port")
	logType := flag.String("log-type", v.GetString("log_type"), "Logger type (e.g., slog)")
	logToFile := flag.String("log-to-file", v.GetString("log_to_file"), "Enable logging to file (true/false)")
	logFile := flag.String("log-file", v.GetString("log_file"), "Path to log file")
	flag.Parse()

	rateLimitCfg, err := loadRateLimitConfig(v)
	if err != nil {
		return nil, fmt.Errorf("error loading rate limit config: %w", err)
	}

	balancerCfg, err := loadBalancerConfig(v)
	if err != nil {
		return nil, fmt.Errorf("error loading balancer config: %w", err)
	}

	// Преобразуем строковые значения в нужные типы
	logToFileBool, err := strconv.ParseBool(resolve(v, "log_to_file", *logToFile))
	if err != nil {
		return nil, fmt.Errorf("invalid log_to_file value: %w", err)
	}

	return &Config{
		HTTPPort:  resolve(v, "http_port", *httpPort),
		RateLimit: rateLimitCfg,
		Balancer:  balancerCfg,
		LogType:   resolve(v, "log_type", *logType),
		LogToFile: logToFileBool,
		LogFile:   resolve(v, "log_file", *logFile),
	}, nil
}

// loadRateLimitConfig loads configuration for the rate limiter.
func loadRateLimitConfig(v *viper.Viper) (RateLimitConfig, error) {
	rateLimit := flag.String("rate-limit", v.GetString("rate_limit.enabled"), "Enable rate limiting (true/false)")
	rate := flag.String("rate-limit-rate", v.GetString("rate_limit.default_rate"), "Tokens per second")
	burst := flag.String("rate-limit-burst", v.GetString("rate_limit.default_burst"), "Bucket capacity")
	limiterType := flag.String("rate-limit-type", v.GetString("rate_limit.type"), "Rate limiter algorithm type (e.g., token-bucket)")
	tickerDuration := flag.String("rate-limit-ticker", v.GetString("rate_limit.ticker_duration"), "Ticker interval duration")
	flag.Parse()

	enabled, err := strconv.ParseBool(resolve(v, "rate_limit.enabled", *rateLimit))
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("invalid rate_limit.enabled value: %w", err)
	}

	defaultRate, err := strconv.Atoi(resolve(v, "rate_limit.default_rate", *rate))
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("invalid rate_limit.default_rate value: %w", err)
	}

	defaultBurst, err := strconv.Atoi(resolve(v, "rate_limit.default_burst", *burst))
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("invalid rate_limit.default_burst value: %w", err)
	}

	tickerDur, err := time.ParseDuration(resolve(v, "rate_limit.ticker_duration", *tickerDuration))
	if err != nil {
		return RateLimitConfig{}, fmt.Errorf("invalid rate_limit.ticker_duration value: %w", err)
	}

	return RateLimitConfig{
		Enabled:        enabled,
		DefaultRate:    defaultRate,
		DefaultBurst:   defaultBurst,
		Type:           resolve(v, "rate_limit.type", *limiterType),
		TickerDuration: tickerDur,
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

// setDefaults sets default configuration values for the application.
func setDefaults(v *viper.Viper) {
	v.SetDefault("http_port", "8080")
	v.SetDefault("backends", []string{"http://localhost:8081", "http://localhost:8082"})
	v.SetDefault("log_type", "slog")
	v.SetDefault("log_to_file", "false")
	v.SetDefault("log_file", "logs/balancer.log")
	v.SetDefault("rate_limit.enabled", "false")
	v.SetDefault("rate_limit.default_rate", "10")
	v.SetDefault("rate_limit.default_burst", "20")
	v.SetDefault("rate_limit.type", "token_bucket")
	v.SetDefault("rate_limit.ticker_duration", "1s")
	v.SetDefault("balancer.type", "least_conn")
}
