package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type BackendConfig struct {
	ID         string `json:"id" yaml:"id"`
	Host       string `json:"host" yaml:"host"`
	Port       int    `json:"port" yaml:"port"`
	Weight     int    `json:"weight" yaml:"weight"`
	HealthPath string `json:"health_path" yaml:"health_path"`
}

type HealthCheckConfig struct {
	IntervalMS       int `json:"interval_ms" yaml:"interval_ms"`
	TimeoutMS        int `json:"timeout_ms" yaml:"timeout_ms"`
	FailureThreshold int `json:"failure_threshold" yaml:"failure_threshold"`
	SuccessThreshold int `json:"success_threshold" yaml:"success_threshold"`
}

type RateLimitConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	RPS     int  `json:"rps" yaml:"rps"`
	Burst   int  `json:"burst" yaml:"burst"`
}

type Config struct {
	ListenAddress    string            `json:"listen_address" yaml:"listen_address"`
	AdminAddress     string            `json:"admin_address" yaml:"admin_address"`
	Strategy         string            `json:"strategy" yaml:"strategy"`
	RequestTimeoutMS int               `json:"request_timeout_ms" yaml:"request_timeout_ms"`
	RetryCount       int               `json:"retry_count" yaml:"retry_count"`
	HealthCheck      HealthCheckConfig `json:"health_check" yaml:"health_check"`
	RateLimit        RateLimitConfig   `json:"rate_limit" yaml:"rate_limit"`
	Backends         []BackendConfig   `json:"backends" yaml:"backends"`
}

func Load(path string) (*Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(buf, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(buf, cfg); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format %q", ext)
	}

	applyDefaults(cfg)
	overrideWithEnv(cfg)
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":8080"
	}
	if cfg.AdminAddress == "" {
		cfg.AdminAddress = ":8081"
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "round_robin"
	}
	if cfg.RequestTimeoutMS <= 0 {
		cfg.RequestTimeoutMS = 3000
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 1
	}
	if cfg.HealthCheck.IntervalMS <= 0 {
		cfg.HealthCheck.IntervalMS = 2000
	}
	if cfg.HealthCheck.TimeoutMS <= 0 {
		cfg.HealthCheck.TimeoutMS = 800
	}
	if cfg.HealthCheck.FailureThreshold <= 0 {
		cfg.HealthCheck.FailureThreshold = 2
	}
	if cfg.HealthCheck.SuccessThreshold <= 0 {
		cfg.HealthCheck.SuccessThreshold = 2
	}
	if cfg.RateLimit.RPS <= 0 {
		cfg.RateLimit.RPS = 0
	}
	if cfg.RateLimit.Burst <= 0 {
		cfg.RateLimit.Burst = 100
	}

	for i := range cfg.Backends {
		if cfg.Backends[i].Weight <= 0 {
			cfg.Backends[i].Weight = 1
		}
		if cfg.Backends[i].HealthPath == "" {
			cfg.Backends[i].HealthPath = "/health"
		}
	}
}

func overrideWithEnv(cfg *Config) {
	if v := os.Getenv("LISTEN_PORT"); v != "" {
		cfg.ListenAddress = normalizeAddress(v)
	}
	if v := os.Getenv("ADMIN_PORT"); v != "" {
		cfg.AdminAddress = normalizeAddress(v)
	}
	if v := os.Getenv("STRATEGY"); v != "" {
		cfg.Strategy = strings.ToLower(v)
	}
	if v := os.Getenv("REQUEST_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RequestTimeoutMS = n
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Backends) == 0 {
		return errors.New("at least one backend is required")
	}
	for i, b := range cfg.Backends {
		if b.ID == "" {
			return fmt.Errorf("backend[%d].id is required", i)
		}
		if b.Host == "" {
			return fmt.Errorf("backend[%d].host is required", i)
		}
		if b.Port <= 0 || b.Port > 65535 {
			return fmt.Errorf("backend[%d].port is invalid", i)
		}
	}
	return nil
}

func normalizeAddress(portOrAddr string) string {
	if strings.Contains(portOrAddr, ":") {
		return portOrAddr
	}
	return ":" + portOrAddr
}
