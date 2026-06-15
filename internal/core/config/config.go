// Package config loads all runtime configuration from environment variables
// using only the standard library. There is no config file format and no
// external dependency: every value has a sensible default, and Load reports
// all parse errors at once so a misconfigured deploy fails fast and clearly.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// lookup returns the trimmed value of an env var and whether it was set.
func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

// Config is the fully-parsed, validated runtime configuration. It is built once
// at startup and injected into the app; nothing reads the environment after Load.
type Config struct {
	Env  string // "development" | "production"
	Addr string // public listen address, e.g. ":8080"

	DatabaseURL string

	// pgxpool tuning.
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration

	// http.Server timeouts.
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration

	MaxRequestBytes int64

	// App-level rate limit (defense-in-depth; primary limiting is the CF edge).
	RateLimitEnabled bool
	RateLimitRPS     float64
	RateLimitBurst   int

	// CIDRs whose direct connections may set client-IP headers. Empty = trust none.
	TrustedProxies []netip.Prefix

	GzipEnabled bool

	PprofEnabled bool
	PprofAddr    string
}

// IsProduction reports whether the app runs in production mode, which tightens
// cookie flags (Secure, __Host- prefix) and enables HSTS.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// Load reads and validates configuration from the environment. All defaults are
// safe for local development. Any parse failures are aggregated into a single
// error so the operator sees every problem at once.
func Load() (*Config, error) {
	l := &loader{}
	c := &Config{
		Env:                 l.str("APP_ENV", "development"),
		Addr:                l.str("APP_ADDR", ":8080"),
		DatabaseURL:         l.str("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable"),
		DBMaxConns:          int32(l.intVal("DB_MAX_CONNS", 10)),
		DBMinConns:          int32(l.intVal("DB_MIN_CONNS", 2)),
		DBMaxConnLifetime:   l.dur("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:   l.dur("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBHealthCheckPeriod: l.dur("DB_HEALTH_CHECK_PERIOD", time.Minute),
		ReadTimeout:         l.dur("READ_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout:   l.dur("READ_HEADER_TIMEOUT", 5*time.Second),
		WriteTimeout:        l.dur("WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:         l.dur("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:     l.dur("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxRequestBytes:     l.int64Val("MAX_REQUEST_BYTES", 1<<20),
		RateLimitEnabled:    l.boolVal("RATE_LIMIT_ENABLED", true),
		RateLimitRPS:        l.floatVal("RATE_LIMIT_RPS", 20),
		RateLimitBurst:      l.intVal("RATE_LIMIT_BURST", 40),
		TrustedProxies:      l.cidrs("TRUSTED_PROXIES"),
		GzipEnabled:         l.boolVal("GZIP_ENABLED", true),
		PprofEnabled:        l.boolVal("PPROF_ENABLED", false),
		PprofAddr:           l.str("PPROF_ADDR", "127.0.0.1:6060"),
	}
	if err := l.err(); err != nil {
		return nil, err
	}
	return c, nil
}

// loader accumulates parse errors so Load can report them all together.
type loader struct{ errs []error }

func (l *loader) err() error {
	if len(l.errs) == 0 {
		return nil
	}
	msgs := make([]string, len(l.errs))
	for i, e := range l.errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(msgs, "\n  - "))
}

func (l *loader) str(key, def string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return def
}

func (l *loader) intVal(key string, def int) int {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q: not an integer", key, v))
		return def
	}
	return n
}

func (l *loader) int64Val(key string, def int64) int64 {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q: not an integer", key, v))
		return def
	}
	return n
}

func (l *loader) floatVal(key string, def float64) float64 {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q: not a number", key, v))
		return def
	}
	return n
}

func (l *loader) boolVal(key string, def bool) bool {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q: not a boolean", key, v))
		return def
	}
	return b
}

func (l *loader) dur(key string, def time.Duration) time.Duration {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s=%q: not a duration (e.g. 30s, 5m, 1h)", key, v))
		return def
	}
	return d
}

func (l *loader) cidrs(key string) []netip.Prefix {
	v, ok := lookup(key)
	if !ok || v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not a valid CIDR", key, p))
			continue
		}
		out = append(out, prefix)
	}
	return out
}
