// Package config loads db-manager configuration from environment variables.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rate is a fixed-window rate limit: Count events per Per duration.
type Rate struct {
	Count int
	Per   time.Duration
}

// Config holds all server settings.
type Config struct {
	Listen         string
	SessionTTL     time.Duration
	LoginRateLimit Rate
	TrustedServers []string
}

// Load reads configuration via getenv, applying defaults for unset variables
// and validating the values.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Listen:         ":8080",
		SessionTTL:     30 * time.Minute,
		LoginRateLimit: Rate{Count: 10, Per: time.Minute},
	}

	if v := getenv("LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := getenv("SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid SESSION_TTL %q: %w", v, err)
		}
		cfg.SessionTTL = d
	}
	if v := getenv("LOGIN_RATE_LIMIT"); v != "" {
		r, err := parseRate(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid LOGIN_RATE_LIMIT %q: %w", v, err)
		}
		cfg.LoginRateLimit = r
	}
	if v := getenv("TRUSTED_SERVERS"); v != "" {
		cfg.TrustedServers = strings.Split(v, ",")
	}
	return cfg, nil
}

// parseRate parses a rate string of the form "N/unit" where unit is "min" or "s".
func parseRate(s string) (Rate, error) {
	countStr, unit, ok := strings.Cut(s, "/")
	if !ok {
		return Rate{}, fmt.Errorf("expected format N/unit, got %q", s)
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return Rate{}, fmt.Errorf("invalid count %q", countStr)
	}
	if count <= 0 {
		return Rate{}, fmt.Errorf("count must be positive, got %d", count)
	}
	var per time.Duration
	switch unit {
	case "min":
		per = time.Minute
	case "s":
		per = time.Second
	default:
		return Rate{}, fmt.Errorf("unknown unit %q (want min or s)", unit)
	}
	return Rate{Count: count, Per: per}, nil
}
