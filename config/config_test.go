package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			require.NoError(t, err)
			require.Equalf(t, want, cfg.RequireEmailVerification, "SPHERICON_REQUIRE_EMAIL_VERIFICATION=%q: RequireEmailVerification = %v, want %v", value, cfg.RequireEmailVerification, want)
		})
	}
}

func TestConfigLoadRequireEmailVerificationIgnoresSMTPSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SMTP_HOST", "")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.True(t, cfg.RequireEmailVerification, "an unset SMTP host must not turn the gate off")
}

type jwtSecretCase struct {
	name      string
	env       string
	jwtSecret string
}

func loadWithJWTSecret(t *testing.T, tc jwtSecretCase) error {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_JWT_SECRET", tc.jwtSecret)
	_, err := Load(tc.env)
	return err
}

func TestConfigLoad_JWTSecretRefusesAWeakOneOutsideDevelopment(t *testing.T) {
	for _, tc := range []jwtSecretCase{
		{name: "production without secret", env: "production", jwtSecret: ""},
		{name: "production short secret", env: "production", jwtSecret: "s3cret"},
		{name: "production 31 chars", env: "production", jwtSecret: strongSecret[:31]},
		{name: "production dev default", env: "production", jwtSecret: "dev-secret-change-in-production"},
		{name: "production change-me padded", env: "production", jwtSecret: "change-me-change-me-change-me-change-me"},
		{name: "production placeholder case-insensitive", env: "production", jwtSecret: "CHANGEME-CHANGEME-CHANGEME-CHANGEME"},
		{name: "staging short secret", env: "staging", jwtSecret: "s3cret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := loadWithJWTSecret(t, tc)

			require.Errorf(t, err, "Load(%q) with secret %q", tc.env, tc.jwtSecret)
		})
	}
}

func TestConfigLoad_JWTSecretAcceptsAStrongOneAndAnyInDevelopment(t *testing.T) {
	for _, tc := range []jwtSecretCase{
		{name: "production strong secret", env: "production", jwtSecret: strongSecret},
		{name: "development without secret", env: "development", jwtSecret: ""},
		{name: "development placeholder", env: "development", jwtSecret: "dev-secret-change-in-production"},
		{name: "test without secret", env: "test", jwtSecret: ""},
		{name: "empty env without secret", env: "", jwtSecret: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := loadWithJWTSecret(t, tc)

			require.NoErrorf(t, err, "Load(%q) with secret %q", tc.env, tc.jwtSecret)
		})
	}
}

func TestConfigLoadDBPoolDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	want := DBPool{MaxOpenConns: 15, MaxIdleConns: 15, ConnMaxLifetime: 30 * time.Minute, PGXMaxConns: 25}
	require.Equalf(t, want, cfg.DBPool, "defaults = %+v, want %+v", cfg.DBPool, want)
}

func TestConfigLoadDBPoolFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "7")
	t.Setenv("SPHERICON_DB_MAX_IDLE_CONNS", "3")
	t.Setenv("SPHERICON_DB_CONN_MAX_LIFETIME", "5m")
	t.Setenv("SPHERICON_PGX_MAX_CONNS", "11")
	cfg, err := Load("test")
	require.NoError(t, err)
	want := DBPool{MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetime: 5 * time.Minute, PGXMaxConns: 11}
	require.Equalf(t, want, cfg.DBPool, "pool = %+v, want %+v", cfg.DBPool, want)
}

func TestConfigLoadDBPoolIdleFollowsOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "9")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, 9, cfg.DBPool.MaxIdleConns, "idle = %d, want 9", cfg.DBPool.MaxIdleConns)
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
			_, err := Load("test")
			require.Error(t, err, "want error")
		})
	}
}

func TestConfigLoadBodyLimitDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, (BodyLimits{Default: 1 << 20, Collect: 500 << 10, CollectEvent: 32 << 10}), cfg.BodyLimits, "defaults = %+v", cfg.BodyLimits)
}

func TestConfigLoadOutboxFloor(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, 7*24*time.Hour, cfg.OutboxFloor, "default OutboxFloor = %v, want 7 days", cfg.OutboxFloor)

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "14")
	cfg, err = Load("test")
	require.NoError(t, err)
	require.Equalf(t, 14*24*time.Hour, cfg.OutboxFloor, "OutboxFloor = %v, want 14 days", cfg.OutboxFloor)

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "-1")
	_, err = Load("test")
	require.Error(t, err, "a negative floor must be rejected")
}

func TestConfigLoadEventsRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, 400*24*time.Hour, cfg.EventsRetention, "default EventsRetention = %v, want 400 days", cfg.EventsRetention)

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "0")
	cfg, err = Load("test")
	require.NoError(t, err)
	require.Equalf(t, time.Duration(0), cfg.EventsRetention, "EventsRetention = %v, want 0 (disabled)", cfg.EventsRetention)

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "-1")
	_, err = Load("test")
	require.Error(t, err, "a negative retention must be rejected")
}

func TestConfigLoadUnverifiedUserRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, 7*24*time.Hour, cfg.UnverifiedUserRetention, "default UnverifiedUserRetention = %v, want 7 days", cfg.UnverifiedUserRetention)

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "0")
	cfg, err = Load("test")
	require.NoError(t, err)
	require.Equalf(t, time.Duration(0), cfg.UnverifiedUserRetention, "UnverifiedUserRetention = %v, want 0 (disabled)", cfg.UnverifiedUserRetention)

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "-1")
	_, err = Load("test")
	require.Error(t, err, "a negative retention must be rejected")
}

type addrCase struct {
	name string
	port string
	addr string
}

func loadWithMetricsAddr(t *testing.T, tc addrCase) (*Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", tc.port)
	t.Setenv("SPHERICON_METRICS_ADDR", tc.addr)
	return Load("test")
}

func TestConfigLoadMetricsAddrAcceptsAnAddressApartFromPORT(t *testing.T) {
	for _, tc := range []addrCase{
		{name: "empty disables the listener", port: "3000", addr: ""},
		{name: "distinct host and port", port: "3000", addr: "127.0.0.1:9090"},
		{name: "all interfaces", port: "3000", addr: "0.0.0.0:9090"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadWithMetricsAddr(t, tc)

			require.NoErrorf(t, err, "SPHERICON_METRICS_ADDR=%q PORT=%s", tc.addr, tc.port)
			require.Equal(t, tc.addr, cfg.MetricsAddr)
		})
	}
}

func TestConfigLoadMetricsAddrRefusesACollisionOrAMalformedOne(t *testing.T) {
	for _, tc := range []addrCase{
		{name: "same port as PORT", port: "3000", addr: "127.0.0.1:3000"},
		{name: "same port with another host", port: "3000", addr: "0.0.0.0:3000"},
		{name: "same port with an empty host", port: "3000", addr: ":3000"},
		{name: "missing port", port: "3000", addr: "127.0.0.1"},
		{name: "non-numeric port", port: "3000", addr: "127.0.0.1:metrics"},
		{name: "port out of range", port: "3000", addr: "127.0.0.1:70000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadWithMetricsAddr(t, tc)

			require.Errorf(t, err, "SPHERICON_METRICS_ADDR=%q PORT=%s", tc.addr, tc.port)
		})
	}
}

func TestConfigLoadE2EProfileDisablesRateLimits(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("e2e")
	require.NoError(t, err)
	require.Equalf(t, (RateLimits{}), cfg.RateLimits, "e2e rate limits = %+v, want all disabled", cfg.RateLimits)
	require.False(t, cfg.IsDev, "e2e must not enable the dev flag (dev DKIM lookup); the harness injects its own")
	prod, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, DefaultRateLimits, prod.RateLimits, "test profile must keep default limits, got %+v", prod.RateLimits)
}

func TestConfigLoadE2EProfileHonoursExplicitRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_RATE_LIMIT_API_PER_MINUTE", "5")
	cfg, err := Load("e2e")
	require.NoError(t, err)
	require.Equalf(t, 5, cfg.RateLimits.APIPerMinute, "APIPerMinute = %d, want explicit 5", cfg.RateLimits.APIPerMinute)
}

func TestConfigLoadE2EProfileIgnoresMetricsAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_METRICS_ADDR", "127.0.0.1:9090")
	cfg, err := Load("e2e")
	require.NoError(t, err)
	require.Emptyf(t, cfg.MetricsAddr, "e2e MetricsAddr = %q, want empty so runs never collide on a port", cfg.MetricsAddr)
	dev, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, "127.0.0.1:9090", dev.MetricsAddr, "test profile MetricsAddr = %q, want the configured one", dev.MetricsAddr)
}

type internalAddrCase struct {
	name    string
	port    string
	metrics string
	addr    string
}

func loadWithInternalAddr(t *testing.T, tc internalAddrCase) (*Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", tc.port)
	t.Setenv("SPHERICON_METRICS_ADDR", tc.metrics)
	t.Setenv("SPHERICON_INTERNAL_ADDR", tc.addr)
	return Load("test")
}

func TestConfigLoadInternalAddrAcceptsAnAddressApartFromTheOtherListeners(t *testing.T) {
	for _, tc := range []internalAddrCase{
		{name: "empty disables the listener", port: "3000", addr: ""},
		{name: "distinct host and port", port: "3000", addr: "127.0.0.1:3400"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadWithInternalAddr(t, tc)

			require.NoErrorf(t, err, "SPHERICON_INTERNAL_ADDR=%q PORT=%s", tc.addr, tc.port)
			require.Equal(t, tc.addr, cfg.InternalAddr)
		})
	}
}

func TestConfigLoadInternalAddrRefusesACollisionOrAMalformedOne(t *testing.T) {
	for _, tc := range []internalAddrCase{
		{name: "malformed", port: "3000", addr: "3400"},
		{name: "same port as PORT", port: "3000", addr: "127.0.0.1:3000"},
		{name: "same port with an empty host", port: "3000", addr: ":3000"},
		{name: "same address as SPHERICON_METRICS_ADDR", port: "3000", metrics: "127.0.0.1:9090", addr: "127.0.0.1:9090"},
		{name: "same port as SPHERICON_METRICS_ADDR on another host", port: "3000", metrics: "127.0.0.1:9090", addr: "0.0.0.0:9090"},
		{name: "port out of range", port: "3000", addr: "127.0.0.1:70000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadWithInternalAddr(t, tc)

			require.Errorf(t, err, "SPHERICON_INTERNAL_ADDR=%q PORT=%s", tc.addr, tc.port)
		})
	}
}

func TestConfigLoadE2EProfileIgnoresInternalAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_INTERNAL_ADDR", "127.0.0.1:3400")
	cfg, err := Load("e2e")
	require.NoError(t, err)
	require.Emptyf(t, cfg.InternalAddr, "e2e InternalAddr = %q, want empty so runs never collide on a port", cfg.InternalAddr)
}

func TestConfigLoadSMTPAllowPrivateHostsFollowsTheEnvironmentUnlessSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	require.True(t, cfg.SMTPAllowPrivateHosts, "a non-production environment allows private SMTP hosts by default")

	t.Setenv("SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS", "false")
	cfg, err = Load("test")
	require.NoError(t, err)
	require.False(t, cfg.SMTPAllowPrivateHosts, "an explicit false must win over the environment default")
}

func TestConfigLoadAbuseLimitsDefaultToTheADR(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := Load("test")
	require.NoError(t, err)
	assert.Equal(t, 1000, cfg.AbuseVolumeFloor)
	assert.InDelta(t, 0.1, cfg.AbuseWarnComplaintPercent, 1e-9)
	assert.InDelta(t, 5, cfg.AbuseWarnBouncePercent, 1e-9)
	assert.InDelta(t, 0.3, cfg.AbuseFreezeComplaintPercent, 1e-9)
	assert.InDelta(t, 10, cfg.AbuseFreezeBouncePercent, 1e-9)
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
			_, err := Load("test")
			require.Error(t, err, "want an error")
		})
	}
}

func TestConfigLoadOutboundAllowIsEmptyByDefaultAndRejectsMalformedValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	_, err := Load("test")
	require.NoError(t, err)

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n:5678,node-red.lan:1880")
	_, err = Load("test")
	require.NoErrorf(t, err, "a list of host:port pairs must load: %v", err)

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n")
	_, err = Load("test")
	require.ErrorContains(t, err, "SPHERICON_OUTBOUND_ALLOW", "a malformed value must fail the load naming the variable")
}

// secretFile writes content to a temp file and returns its path.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
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
	require.NoErrorf(t, err, "Load: %v", err)
	for name, got := range map[string][2]string{
		"DatabaseURL":   {cfg.DatabaseURL, "postgres://from-file"},
		"JWTSecret":     {cfg.JWTSecret, strongSecret},
		"EncryptionKey": {cfg.EncryptionKey, "keyset"},
		"SMTPPass":      {cfg.SMTPPass, "smtp-pass"},
		"LicenseKey":    {cfg.LicenseKey, "license.jwt"},
	} {
		assert.Equalf(t, got[1], got[0], "%s = %q, want %q", name, got[0], got[1])
	}
}

func TestConfigLoadExplicitSecretWinsOverFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://explicit")
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "missing"))

	cfg, err := Load("test")
	require.NoErrorf(t, err, "Load: %v", err)
	assert.Equalf(t, "postgres://explicit", cfg.DatabaseURL, "DatabaseURL = %q, want the explicit value", cfg.DatabaseURL)
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
			require.ErrorContains(t, err, "SPHERICON_ENCRYPTION_KEY_FILE")
		})
	}
}

func loadWithSecondarySecret(t *testing.T, env, secondary string) (*Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_JWT_SECRET", strongSecret)
	t.Setenv("SPHERICON_JWT_SECRET_SECONDARY", secondary)
	return Load(env)
}

func TestConfigLoadJWTSecretSecondaryIsVerifyOnlyAndOptional(t *testing.T) {
	other := strings.Repeat("m3", 16)
	for name, tc := range map[string]struct {
		env, secondary string
		wantVerify     []string
	}{
		"none":                   {env: "production", secondary: "", wantVerify: nil},
		"a strong second secret": {env: "production", secondary: other, wantVerify: []string{other}},
		"development takes any":  {env: "development", secondary: "short", wantVerify: []string{"short"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadWithSecondarySecret(t, tc.env, tc.secondary)

			require.NoError(t, err)
			assert.Equal(t, tc.secondary, cfg.JWTSecretSecondary)
			assert.ElementsMatch(t, tc.wantVerify, cfg.JWTVerifySecrets())
		})
	}
}

func TestConfigLoadJWTSecretSecondaryRefusesAWeakOrRepeatedOneInProduction(t *testing.T) {
	for name, secondary := range map[string]string{
		"the same secret twice":       strongSecret,
		"a short second secret":       "short",
		"a placeholder second secret": "change-me-change-me-change-me-change-me",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadWithSecondarySecret(t, "production", secondary)

			require.Error(t, err)
		})
	}
}

func TestConfigLoadEventLiveWindow(t *testing.T) {
	cfg, err := Load("test")
	require.NoError(t, err)
	require.Equalf(t, 72*time.Hour, cfg.EventLiveWindow, "default EventLiveWindow = %v, want 72h", cfg.EventLiveWindow)

	t.Setenv("SPHERICON_EVENT_LIVE_WINDOW_HOURS", "0")
	_, err = Load("test")
	require.Error(t, err, "a live window under an hour must be rejected: it cannot be switched off")
}
