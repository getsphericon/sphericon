package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var strongSecret = strings.Repeat("k7", 16)

func TestConfigLoadRequireEmailVerificationOnlyAnExplicitFalseTurnsItOff(t *testing.T) {
	cases := map[string]bool{
		"":      true,
		"true":  true,
		"false": false,
		"0":     true,
		"no":    true,
		"off":   true,
		"False": true,
		"FALSE": true,
		"junk":  true,
	}
	for value, want := range cases {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("SPHERICON_REQUIRE_EMAIL_VERIFICATION", value)
			cfg, err := Load("test")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RequireEmailVerification != want {
				t.Fatalf("SPHERICON_REQUIRE_EMAIL_VERIFICATION=%q: RequireEmailVerification = %v, want %v", value, cfg.RequireEmailVerification, want)
			}
		})
	}
}

func TestConfigLoadRequireEmailVerificationIgnoresSMTPSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SMTP_HOST", "")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RequireEmailVerification {
		t.Fatal("an unset SMTP host must not turn the gate off")
	}
}

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
			t.Setenv("SPHERICON_JWT_SECRET", tc.jwtSecret)
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
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "7")
	t.Setenv("SPHERICON_DB_MAX_IDLE_CONNS", "3")
	t.Setenv("SPHERICON_DB_CONN_MAX_LIFETIME", "5m")
	t.Setenv("SPHERICON_PGX_MAX_CONNS", "11")
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
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "9")
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
		"zero open":     {"SPHERICON_DB_MAX_OPEN_CONNS": "0"},
		"idle > open":   {"SPHERICON_DB_MAX_OPEN_CONNS": "2", "SPHERICON_DB_MAX_IDLE_CONNS": "3"},
		"zero lifetime": {"SPHERICON_DB_CONN_MAX_LIFETIME": "0s"},
		"zero pgx":      {"SPHERICON_PGX_MAX_CONNS": "0"},
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

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "14")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OutboxFloor != 14*24*time.Hour {
		t.Fatalf("OutboxFloor = %v, want 14 days", cfg.OutboxFloor)
	}

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "-1")
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

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "0")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventsRetention != 0 {
		t.Fatalf("EventsRetention = %v, want 0 (disabled)", cfg.EventsRetention)
	}

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "-1")
	if _, err := Load("test"); err == nil {
		t.Fatal("a negative retention must be rejected")
	}
}

func TestConfigLoadUnverifiedUserRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnverifiedUserRetention != 7*24*time.Hour {
		t.Fatalf("default UnverifiedUserRetention = %v, want 7 days", cfg.UnverifiedUserRetention)
	}

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "0")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnverifiedUserRetention != 0 {
		t.Fatalf("UnverifiedUserRetention = %v, want 0 (disabled)", cfg.UnverifiedUserRetention)
	}

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "-1")
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
			t.Setenv("SPHERICON_METRICS_ADDR", tc.addr)
			cfg, err := Load("test")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SPHERICON_METRICS_ADDR=%q PORT=%s: want error, got nil", tc.addr, tc.port)
				}
				return
			}
			if err != nil {
				t.Fatalf("SPHERICON_METRICS_ADDR=%q PORT=%s: unexpected error: %v", tc.addr, tc.port, err)
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
	t.Setenv("SPHERICON_RATE_LIMIT_API_PER_MINUTE", "5")
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
	t.Setenv("SPHERICON_METRICS_ADDR", "127.0.0.1:9090")
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
		{name: "same address as SPHERICON_METRICS_ADDR", port: "3000", metrics: "127.0.0.1:9090", addr: "127.0.0.1:9090", wantErr: true},
		{name: "same port as SPHERICON_METRICS_ADDR on another host", port: "3000", metrics: "127.0.0.1:9090", addr: "0.0.0.0:9090", wantErr: true},
		{name: "port out of range", port: "3000", addr: "127.0.0.1:70000", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("PORT", tc.port)
			t.Setenv("SPHERICON_METRICS_ADDR", tc.metrics)
			t.Setenv("SPHERICON_INTERNAL_ADDR", tc.addr)
			cfg, err := Load("test")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SPHERICON_INTERNAL_ADDR=%q PORT=%s: want error, got nil", tc.addr, tc.port)
				}
				return
			}
			if err != nil {
				t.Fatalf("SPHERICON_INTERNAL_ADDR=%q PORT=%s: unexpected error: %v", tc.addr, tc.port, err)
			}
			if cfg.InternalAddr != tc.addr {
				t.Fatalf("InternalAddr = %q, want %q", cfg.InternalAddr, tc.addr)
			}
		})
	}
}

func TestConfigLoadE2EProfileIgnoresInternalAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_INTERNAL_ADDR", "127.0.0.1:3400")
	cfg, err := Load("e2e")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InternalAddr != "" {
		t.Fatalf("e2e InternalAddr = %q, want empty so runs never collide on a port", cfg.InternalAddr)
	}
}

func TestConfigLoadSMTPAllowPrivateHostsFollowsTheEnvironmentUnlessSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SMTPAllowPrivateHosts {
		t.Fatal("a non-production environment allows private SMTP hosts by default")
	}

	t.Setenv("SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS", "false")
	cfg, err = Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPAllowPrivateHosts {
		t.Fatal("an explicit false must win over the environment default")
	}
}

func TestConfigLoadAbuseLimitsDefaultToTheADR(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AbuseVolumeFloor != 1000 || cfg.AbuseWarnComplaintPercent != 0.1 || cfg.AbuseWarnBouncePercent != 5 ||
		cfg.AbuseFreezeComplaintPercent != 0.3 || cfg.AbuseFreezeBouncePercent != 10 {
		t.Fatalf("limits = %+v", cfg)
	}
}

func TestConfigLoadRejectsAbuseLimitsThatCannotWork(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"zero floor":                  {"SPHERICON_ABUSE_VOLUME_FLOOR": "0"},
		"warning at the freeze":       {"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT": "0.3"},
		"bounce warning above freeze": {"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT": "11"},
		"freeze above 100":            {"SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT": "101"},
		"zero warning":                {"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load("test"); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestConfigLoadOutboundAllowIsEmptyByDefaultAndRejectsMalformedValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	if _, err := Load("test"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n:5678,node-red.lan:1880")
	if _, err := Load("test"); err != nil {
		t.Fatalf("a list of host:port pairs must load: %v", err)
	}

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n")
	_, err := Load("test")
	if err == nil || !strings.Contains(err.Error(), "SPHERICON_OUTBOUND_ALLOW") {
		t.Fatalf("a malformed value must fail the load naming the variable, got %v", err)
	}
}

// secretFile writes content to a temp file and returns its path.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigLoadSecretFromFile(t *testing.T) {
	for _, name := range secretVars {
		t.Setenv(name, "")
	}
	t.Setenv("DATABASE_URL_FILE", secretFile(t, "postgres://from-file\n"))
	t.Setenv("SPHERICON_JWT_SECRET_FILE", secretFile(t, strongSecret+"\r\n"))
	t.Setenv("SPHERICON_ENCRYPTION_KEY_FILE", secretFile(t, "keyset"))
	t.Setenv("SPHERICON_SMTP_PASS_FILE", secretFile(t, "smtp-pass\n"))
	t.Setenv("SPHERICON_LICENSE_KEY_FILE", secretFile(t, "license.jwt\n"))

	// "staging" has no .env file, unlike "test", whose .env.test sets the same variables.
	cfg, err := Load("staging")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for name, got := range map[string][2]string{
		"DatabaseURL":   {cfg.DatabaseURL, "postgres://from-file"},
		"JWTSecret":     {cfg.JWTSecret, strongSecret},
		"EncryptionKey": {cfg.EncryptionKey, "keyset"},
		"SMTPPass":      {cfg.SMTPPass, "smtp-pass"},
		"LicenseKey":    {cfg.LicenseKey, "license.jwt"},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %q, want %q", name, got[0], got[1])
		}
	}
}

func TestConfigLoadExplicitSecretWinsOverFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://explicit")
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "missing"))

	cfg, err := Load("test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://explicit" {
		t.Errorf("DatabaseURL = %q, want the explicit value", cfg.DatabaseURL)
	}
}

func TestConfigLoadSecretFileMustBeReadableAndNotEmpty(t *testing.T) {
	cases := map[string]string{
		"missing": filepath.Join(t.TempDir(), "missing"),
		"empty":   secretFile(t, ""),
		"newline": secretFile(t, "\n"),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("SPHERICON_JWT_SECRET", strongSecret)
			t.Setenv("SPHERICON_ENCRYPTION_KEY", "")
			t.Setenv("SPHERICON_ENCRYPTION_KEY_FILE", path)

			_, err := Load("staging")
			if err == nil || !strings.Contains(err.Error(), "SPHERICON_ENCRYPTION_KEY_FILE") {
				t.Fatalf("Load error = %v, want one naming SPHERICON_ENCRYPTION_KEY_FILE", err)
			}
		})
	}
}

func TestConfigLoadJWTSecretSecondary(t *testing.T) {
	other := strings.Repeat("m3", 16)
	for name, tc := range map[string]struct {
		env, secondary string
		wantErr        bool
	}{
		"none":                        {env: "production"},
		"a strong second secret":      {env: "production", secondary: other},
		"the same secret twice":       {env: "production", secondary: strongSecret, wantErr: true},
		"a short second secret":       {env: "production", secondary: "short", wantErr: true},
		"a placeholder second secret": {env: "production", secondary: "change-me-change-me-change-me-change-me", wantErr: true},
		"development takes any":       {env: "development", secondary: "short"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("SPHERICON_JWT_SECRET", strongSecret)
			t.Setenv("SPHERICON_JWT_SECRET_SECONDARY", tc.secondary)
			cfg, err := Load(tc.env)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.JWTSecretSecondary != tc.secondary {
				t.Fatalf("JWTSecretSecondary = %q, want %q", cfg.JWTSecretSecondary, tc.secondary)
			}
			want := 0
			if tc.secondary != "" {
				want = 1
			}
			if got := cfg.JWTVerifySecrets(); len(got) != want || (want == 1 && got[0] != tc.secondary) {
				t.Fatalf("JWTVerifySecrets() = %v", got)
			}
		})
	}
}

func TestConfigLoadEventLiveWindow(t *testing.T) {
	cfg, err := Load("test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventLiveWindow != 72*time.Hour {
		t.Fatalf("default EventLiveWindow = %v, want 72h", cfg.EventLiveWindow)
	}

	t.Setenv("SPHERICON_EVENT_LIVE_WINDOW_HOURS", "0")
	if _, err := Load("test"); err == nil {
		t.Fatal("a live window under an hour must be rejected: it cannot be switched off")
	}
}
