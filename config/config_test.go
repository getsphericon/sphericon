package config

import (
	"strings"
	"testing"
	"time"
)

var strongSecret = strings.Repeat("k7", 16)

func TestConfigLoad_JWTSecret(t *testing.T) {
	cases := []struct {
		name      string
		env       string
		jwtSecret string
		wantErr   bool
	}{
		{name: "production without secret fails", env: "production", jwtSecret: "", wantErr: true},
		{name: "production short secret fails", env: "production", jwtSecret: "s3cret", wantErr: true},
		{name: "production 31 chars fails", env: "production", jwtSecret: strongSecret[:31], wantErr: true},
		{name: "production dev default fails", env: "production", jwtSecret: "dev-secret-change-in-production", wantErr: true},
		{name: "production change-me padded fails", env: "production", jwtSecret: "change-me-change-me-change-me-change-me", wantErr: true},
		{name: "production placeholder case-insensitive fails", env: "production", jwtSecret: "CHANGEME-CHANGEME-CHANGEME-CHANGEME", wantErr: true},
		{name: "production strong secret ok", env: "production", jwtSecret: strongSecret, wantErr: false},
		{name: "staging short secret fails", env: "staging", jwtSecret: "s3cret", wantErr: true},
		{name: "development without secret ok", env: "development", jwtSecret: "", wantErr: false},
		{name: "development placeholder ok", env: "development", jwtSecret: "dev-secret-change-in-production", wantErr: false},
		{name: "test without secret ok", env: "test", jwtSecret: "", wantErr: false},
		{name: "empty env without secret ok", env: "", jwtSecret: "", wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("JWT_SECRET", tc.jwtSecret)
			_, err := Load(tc.env)
			if tc.wantErr && err == nil {
				t.Fatalf("Load(%q) with secret %q: want error, got nil", tc.env, tc.jwtSecret)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Load(%q) with secret %q: unexpected error: %v", tc.env, tc.jwtSecret, err)
			}
		})
	}
}

func TestConfigLoadDBPoolDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	want := DBPool{MaxOpenConns: 15, MaxIdleConns: 15, ConnMaxLifetime: 30 * time.Minute, PGXMaxConns: 25}
	if cfg.DBPool != want {
		t.Fatalf("defaults = %+v, want %+v", cfg.DBPool, want)
	}
}

func TestConfigLoadDBPoolFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DB_MAX_OPEN_CONNS", "7")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_CONN_MAX_LIFETIME", "5m")
	t.Setenv("PGX_MAX_CONNS", "11")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	want := DBPool{MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetime: 5 * time.Minute, PGXMaxConns: 11}
	if cfg.DBPool != want {
		t.Fatalf("pool = %+v, want %+v", cfg.DBPool, want)
	}
}

func TestConfigLoadDBPoolIdleFollowsOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DB_MAX_OPEN_CONNS", "9")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPool.MaxIdleConns != 9 {
		t.Fatalf("idle = %d, want 9", cfg.DBPool.MaxIdleConns)
	}
}

func TestConfigLoadDBPoolRejectsInvalid(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"zero open":     {"DB_MAX_OPEN_CONNS": "0"},
		"idle > open":   {"DB_MAX_OPEN_CONNS": "2", "DB_MAX_IDLE_CONNS": "3"},
		"zero lifetime": {"DB_CONN_MAX_LIFETIME": "0s"},
		"zero pgx":      {"PGX_MAX_CONNS": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load("test"); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestConfigLoadBodyLimitDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BodyLimits != (BodyLimits{Default: 1 << 20, Collect: 500 << 10, CollectEvent: 32 << 10}) {
		t.Fatalf("defaults = %+v", cfg.BodyLimits)
	}
}

func TestConfigLoadOutboxFloor(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OutboxFloor != 7*24*time.Hour {
		t.Fatalf("default OutboxFloor = %v, want 7 days", cfg.OutboxFloor)
	}

	t.Setenv("OUTBOX_RETENTION_FLOOR_DAYS", "14")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OutboxFloor != 14*24*time.Hour {
		t.Fatalf("OutboxFloor = %v, want 14 days", cfg.OutboxFloor)
	}

	t.Setenv("OUTBOX_RETENTION_FLOOR_DAYS", "-1")
	if _, err := Load("test"); err == nil {
		t.Fatal("a negative floor must be rejected")
	}
}

func TestConfigLoadEventsRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventsRetention != 400*24*time.Hour {
		t.Fatalf("default EventsRetention = %v, want 400 days", cfg.EventsRetention)
	}

	t.Setenv("EVENTS_RETENTION_DAYS", "0")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventsRetention != 0 {
		t.Fatalf("EventsRetention = %v, want 0 (disabled)", cfg.EventsRetention)
	}

	t.Setenv("EVENTS_RETENTION_DAYS", "-1")
	if _, err := Load("test"); err == nil {
		t.Fatal("a negative retention must be rejected")
	}
}

func TestConfigLoadMetricsAddr(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		addr    string
		wantErr bool
	}{
		{name: "empty disables the listener", port: "3000", addr: ""},
		{name: "distinct host and port", port: "3000", addr: "127.0.0.1:9090"},
		{name: "all interfaces", port: "3000", addr: "0.0.0.0:9090"},
		{name: "same port as PORT", port: "3000", addr: "127.0.0.1:3000", wantErr: true},
		{name: "same port with another host", port: "3000", addr: "0.0.0.0:3000", wantErr: true},
		{name: "same port with an empty host", port: "3000", addr: ":3000", wantErr: true},
		{name: "missing port", port: "3000", addr: "127.0.0.1", wantErr: true},
		{name: "non-numeric port", port: "3000", addr: "127.0.0.1:metrics", wantErr: true},
		{name: "port out of range", port: "3000", addr: "127.0.0.1:70000", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("PORT", tc.port)
			t.Setenv("METRICS_ADDR", tc.addr)
			cfg, err := Load("test")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("METRICS_ADDR=%q PORT=%s: want error, got nil", tc.addr, tc.port)
				}
				return
			}
			if err != nil {
				t.Fatalf("METRICS_ADDR=%q PORT=%s: unexpected error: %v", tc.addr, tc.port, err)
			}
			if cfg.MetricsAddr != tc.addr {
				t.Fatalf("MetricsAddr = %q, want %q", cfg.MetricsAddr, tc.addr)
			}
		})
	}
}

func TestConfigLoadE2EProfileDisablesRateLimits(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("e2e")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimits != (RateLimits{}) {
		t.Fatalf("e2e rate limits = %+v, want all disabled", cfg.RateLimits)
	}
	if cfg.IsDev {
		t.Fatal("e2e must not enable the dev flag (dev DKIM lookup); the harness injects its own")
	}
	prod, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if prod.RateLimits != DefaultRateLimits {
		t.Fatalf("test profile must keep default limits, got %+v", prod.RateLimits)
	}
}

func TestConfigLoadE2EProfileHonoursExplicitRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("RATE_LIMIT_API_PER_MINUTE", "5")
	cfg, err := Load("e2e")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimits.APIPerMinute != 5 {
		t.Fatalf("APIPerMinute = %d, want explicit 5", cfg.RateLimits.APIPerMinute)
	}
}

func TestConfigLoadE2EProfileIgnoresMetricsAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("METRICS_ADDR", "127.0.0.1:9090")
	cfg, err := Load("e2e")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MetricsAddr != "" {
		t.Fatalf("e2e MetricsAddr = %q, want empty so runs never collide on a port", cfg.MetricsAddr)
	}
	dev, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if dev.MetricsAddr != "127.0.0.1:9090" {
		t.Fatalf("test profile MetricsAddr = %q, want the configured one", dev.MetricsAddr)
	}
}

func TestConfigLoadInternalAddr(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		metrics string
		addr    string
		wantErr bool
	}{
		{name: "empty disables the listener", port: "3000", addr: ""},
		{name: "distinct host and port", port: "3000", addr: "127.0.0.1:3400"},
		{name: "malformed", port: "3000", addr: "3400", wantErr: true},
		{name: "same port as PORT", port: "3000", addr: "127.0.0.1:3000", wantErr: true},
		{name: "same port with an empty host", port: "3000", addr: ":3000", wantErr: true},
		{name: "same address as METRICS_ADDR", port: "3000", metrics: "127.0.0.1:9090", addr: "127.0.0.1:9090", wantErr: true},
		{name: "same port as METRICS_ADDR on another host", port: "3000", metrics: "127.0.0.1:9090", addr: "0.0.0.0:9090", wantErr: true},
		{name: "port out of range", port: "3000", addr: "127.0.0.1:70000", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("PORT", tc.port)
			t.Setenv("METRICS_ADDR", tc.metrics)
			t.Setenv("INTERNAL_ADDR", tc.addr)
			cfg, err := Load("test")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("INTERNAL_ADDR=%q PORT=%s: want error, got nil", tc.addr, tc.port)
				}
				return
			}
			if err != nil {
				t.Fatalf("INTERNAL_ADDR=%q PORT=%s: unexpected error: %v", tc.addr, tc.port, err)
			}
			if cfg.InternalAddr != tc.addr {
				t.Fatalf("InternalAddr = %q, want %q", cfg.InternalAddr, tc.addr)
			}
		})
	}
}

func TestConfigLoadE2EProfileIgnoresInternalAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("INTERNAL_ADDR", "127.0.0.1:3400")
	cfg, err := Load("e2e")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InternalAddr != "" {
		t.Fatalf("e2e InternalAddr = %q, want empty so runs never collide on a port", cfg.InternalAddr)
	}
}
