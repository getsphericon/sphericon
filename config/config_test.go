package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

var strongSecret = strings.Repeat("k7", 16)

func TestConfigLoadModeIsSelfHostedUnlessSaaS(t *testing.T) {
	cases := map[string]config.Mode{
		"":           config.ModeSelfHosted,
		"selfhosted": config.ModeSelfHosted,
		"SaaS":       config.ModeSaaS,
	}
	for value, want := range cases {
		got, err := config.ParseMode(value)
		require.NoError(t, err, "SPHERICON_MODE=%q", value)
		require.Equal(t, want, got, "SPHERICON_MODE=%q", value)
	}

	for _, junk := range []string{"self-hosted", "cloud", "true", "saass"} {
		_, err := config.ParseMode(junk)
		require.ErrorContains(t, err, "SPHERICON_MODE", "SPHERICON_MODE=%q must not start an Instance", junk)
	}
}

func TestConfigLoadModeDecidesWhatDiffers(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_MODE", "")

	selfHosted, err := config.Load("test")
	require.NoError(t, err)
	require.Equal(t, config.ModeSelfHosted, selfHosted.Mode)
	require.False(t, selfHosted.RequireEmailVerification, "selfhosted does not require email verification")
	require.False(t, selfHosted.SignupSurvey, "selfhosted has no Sign-up survey")
	require.True(t, selfHosted.ReleaseCheck, "selfhosted checks for a release")
	require.Nil(t, selfHosted.Telemetry, "selfhosted leaves the Telemetry report to the Owner")

	selfHosted.ApplyMode(config.ModeSaaS)
	require.True(t, selfHosted.RequireEmailVerification, "saas requires email verification")
	require.True(t, selfHosted.SignupSurvey, "saas asks the Sign-up survey")
	require.False(t, selfHosted.ReleaseCheck, "saas makes no Release check")
	require.NotNil(t, selfHosted.Telemetry)
	require.False(t, *selfHosted.Telemetry, "saas keeps the Telemetry report off")
}

func TestConfigLoadSaaSRequiresWhatOnlyADeploymentGives(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_MODE", "saas")
	t.Setenv("PORT", "3000")
	t.Setenv("SPHERICON_METRICS_ADDR", "")
	t.Setenv("SPHERICON_STAFF_JWT_SECRET", "")
	t.Setenv("SPHERICON_INTERNAL_ADDR", "")
	t.Setenv("SPHERICON_TRACKING_CNAME_TARGET", "")
	t.Setenv("SPHERICON_SMTP_HOST", "")

	for _, step := range []struct{ name, value string }{
		{"SPHERICON_STAFF_JWT_SECRET", "a-staff-secret-of-its-own"},
		{"SPHERICON_INTERNAL_ADDR", "127.0.0.1:3100"},
		{"SPHERICON_TRACKING_CNAME_TARGET", "edge.example.com"},
		{"SPHERICON_SMTP_HOST", "smtp.example.com"},
	} {
		_, err := config.Load("test")
		require.ErrorContains(t, err, step.name, "a saas Instance without %s must not start", step.name)
		t.Setenv(step.name, step.value)
	}

	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equal(t, config.ModeSaaS, cfg.Mode)
}

func TestConfigLoadSentryDSNIsOffUnlessSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SENTRY_DSN", "")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Empty(t, cfg.SentryDSN, "browser reporting must be off by default")

	t.Setenv("SPHERICON_SENTRY_DSN", "  https://key@o1.ingest.sentry.io/2 ")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equal(t, "https://key@o1.ingest.sentry.io/2", cfg.SentryDSN)
}

type jwtSecretCase struct {
	name      string
	env       string
	jwtSecret string
}

// loadWithJWTSecret loads tc.env, which outside the automated profiles also reads a developer's
// own .env: the mode is pinned, as it decides what else a valid config needs.
func loadWithJWTSecret(t *testing.T, tc jwtSecretCase) error {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_MODE", "selfhosted")
	t.Setenv("SPHERICON_JWT_SECRET", tc.jwtSecret)
	_, err := config.Load(tc.env)
	return err
}

func TestConfigLoad_JWTSecretRefusesAWeakOneOutsideDevelopment(t *testing.T) {
	// An unset secret is not a row: outside the test environment a developer's own .env would
	// fill it in. ValidateJWTSecret states that refusal (TestValidateJWTSecretRequiresOne).
	for _, tc := range []jwtSecretCase{
		{name: "production short secret", env: "production", jwtSecret: "s3cret"},
		{name: "production 31 chars", env: "production", jwtSecret: strongSecret[:31]},
		{name: "production dev default", env: "production", jwtSecret: "dev-secret-change-in-production"},
		{name: "production change-me padded", env: "production", jwtSecret: "change-me-change-me-change-me-change-me"},
		{name: "production placeholder case-insensitive", env: "production", jwtSecret: "CHANGEME-CHANGEME-CHANGEME-CHANGEME"},
		{name: "staging short secret", env: "staging", jwtSecret: "s3cret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := loadWithJWTSecret(t, tc)

			require.ErrorContainsf(t, err, "SPHERICON_JWT_SECRET", "Load(%q) with secret %q", tc.env, tc.jwtSecret)
		})
	}
}

func TestValidateJWTSecretRequiresOne(t *testing.T) {
	require.ErrorContains(t, config.ValidateJWTSecret(""), "SPHERICON_JWT_SECRET")
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

func TestConfigTrackerOrigin(t *testing.T) {
	cases := map[string]struct{ trackerURL, want string }{
		"unset falls back to the app origin": {"", "http://localhost:3000"},
		"a host of its own":                  {"https://t.example.com", "https://t.example.com"},
		"a trailing slash is dropped":        {"https://t.example.com/", "https://t.example.com"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{AppURL: "http://localhost:3000", TrackerURL: c.trackerURL}
			require.Equal(t, c.want, cfg.TrackerOrigin())
		})
	}
}

func TestConfigLoadDBPoolDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	want := config.DBPool{MaxOpenConns: 15, MaxIdleConns: 15, ConnMaxLifetime: 30 * time.Minute, RiverMaxConns: 25}
	require.Equalf(t, want, cfg.DBPool, "defaults = %+v, want %+v", cfg.DBPool, want)
}

func TestConfigLoadDBPoolFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "7")
	t.Setenv("SPHERICON_DB_MAX_IDLE_CONNS", "3")
	t.Setenv("SPHERICON_DB_CONN_MAX_LIFETIME", "5m")
	t.Setenv("SPHERICON_RIVER_DB_MAX_CONNS", "11")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	want := config.DBPool{MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetime: 5 * time.Minute, RiverMaxConns: 11}
	require.Equalf(t, want, cfg.DBPool, "pool = %+v, want %+v", cfg.DBPool, want)
}

func TestConfigLoadDBPoolIdleFollowsOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "9")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 9, cfg.DBPool.MaxIdleConns, "idle = %d, want 9", cfg.DBPool.MaxIdleConns)
}

func TestConfigLoadDBPoolRejectsInvalid(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"zero open":     {"SPHERICON_DB_MAX_OPEN_CONNS": "0"},
		"idle > open":   {"SPHERICON_DB_MAX_OPEN_CONNS": "2", "SPHERICON_DB_MAX_IDLE_CONNS": "3"},
		"zero lifetime": {"SPHERICON_DB_CONN_MAX_LIFETIME": "0s"},
		"zero river":    {"SPHERICON_RIVER_DB_MAX_CONNS": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			for k, v := range env {
				t.Setenv(k, v)
			}
			_, err := config.Load("test")
			require.Error(t, err, "want error")
		})
	}
}

func TestConfigLoadBodyLimitDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, (config.BodyLimits{Default: 1 << 20, Collect: 500 << 10, CollectEvent: 32 << 10}), cfg.BodyLimits, "defaults = %+v", cfg.BodyLimits)
}

func TestConfigLoadOutboxFloor(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 7*24*time.Hour, cfg.OutboxFloor, "default OutboxFloor = %v, want 7 days", cfg.OutboxFloor)

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "14")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 14*24*time.Hour, cfg.OutboxFloor, "OutboxFloor = %v, want 14 days", cfg.OutboxFloor)

	t.Setenv("SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS", "-1")
	_, err = config.Load("test")
	require.Error(t, err, "a negative floor must be rejected")
}

func TestConfigLoadEventsRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 400*24*time.Hour, cfg.EventsRetention, "default EventsRetention = %v, want 400 days", cfg.EventsRetention)

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "0")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, time.Duration(0), cfg.EventsRetention, "EventsRetention = %v, want 0 (disabled)", cfg.EventsRetention)

	t.Setenv("SPHERICON_EVENTS_RETENTION_DAYS", "-1")
	_, err = config.Load("test")
	require.Error(t, err, "a negative retention must be rejected")
}

func TestConfigLoadUnverifiedUserRetention(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 7*24*time.Hour, cfg.UnverifiedUserRetention, "default UnverifiedUserRetention = %v, want 7 days", cfg.UnverifiedUserRetention)

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "0")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, time.Duration(0), cfg.UnverifiedUserRetention, "UnverifiedUserRetention = %v, want 0 (disabled)", cfg.UnverifiedUserRetention)

	t.Setenv("SPHERICON_UNVERIFIED_USER_RETENTION_DAYS", "-1")
	_, err = config.Load("test")
	require.Error(t, err, "a negative retention must be rejected")
}

func TestConfigLoadTestInboxSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equal(t, 7*24*time.Hour, cfg.TestInboxRetention, "default retention is 7 days")
	require.Equal(t, 200, cfg.TestInboxMaxMessages, "default number kept is 200")

	t.Setenv("SPHERICON_TEST_INBOX_RETENTION_DAYS", "2")
	t.Setenv("SPHERICON_TEST_INBOX_MAX_MESSAGES", "50")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equal(t, 2*24*time.Hour, cfg.TestInboxRetention)
	require.Equal(t, 50, cfg.TestInboxMaxMessages)
}

func TestConfigLoadRejectsATestInboxThatKeepsNothing(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for name, value := range map[string]string{
		"SPHERICON_TEST_INBOX_RETENTION_DAYS": "0",
		"SPHERICON_TEST_INBOX_MAX_MESSAGES":   "0",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			_, err := config.Load("test")
			require.ErrorContains(t, err, name)
		})
	}
}

func TestConfigLoadSystemEmailReplyTo(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Empty(t, cfg.SystemEmailReplyTo, "unset by default")

	t.Setenv("SPHERICON_SYSTEM_EMAIL_REPLY_TO", " replies@example.com ")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equal(t, "replies@example.com", cfg.SystemEmailReplyTo)
}

func TestConfigLoadRejectsAnInvalidSystemEmailReplyTo(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, value := range []string{"not an address", "Support <help@example.com>", "a@example.com, b@example.com", "help@"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SPHERICON_SYSTEM_EMAIL_REPLY_TO", value)
			_, err := config.Load("test")
			require.ErrorContains(t, err, "SPHERICON_SYSTEM_EMAIL_REPLY_TO")
		})
	}
}

func TestConfigLoadSystemEmailFromAcceptsADisplayName(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equal(t, "noreply@sphericon.localhost", cfg.SystemEmailFrom)
	require.Empty(t, cfg.SystemEmailFromName, "no name unless one is given")

	t.Setenv("SPHERICON_SYSTEM_EMAIL_FROM", `Example Mail <noreply@example.com>`)
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.Equal(t, "noreply@example.com", cfg.SystemEmailFrom, "the bare address, for the envelope and identity checks")
	require.Equal(t, "Example Mail", cfg.SystemEmailFromName)
}

func TestConfigLoadRejectsAnInvalidSystemEmailFrom(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, value := range []string{"not an address", "a@example.com, b@example.com", "help@"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SPHERICON_SYSTEM_EMAIL_FROM", value)
			_, err := config.Load("test")
			require.ErrorContains(t, err, "SPHERICON_SYSTEM_EMAIL_FROM")
		})
	}
}

type addrCase struct {
	name string
	port string
	addr string
}

func loadWithMetricsAddr(t *testing.T, tc addrCase) (*config.Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", tc.port)
	t.Setenv("SPHERICON_METRICS_ADDR", tc.addr)
	return config.Load("test")
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
	cfg, err := config.Load("e2e")
	require.NoError(t, err)
	require.Equalf(t, (config.RateLimits{}), cfg.RateLimits, "e2e rate limits = %+v, want all disabled", cfg.RateLimits)
	require.False(t, cfg.IsDev, "e2e must not enable the dev flag (dev DKIM lookup); the harness injects its own")
	prod, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, config.DefaultRateLimits, prod.RateLimits, "test profile must keep default limits, got %+v", prod.RateLimits)
}

func TestConfigLoadE2EProfileHonoursExplicitRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_RATE_LIMIT_API_PER_MINUTE", "5")
	cfg, err := config.Load("e2e")
	require.NoError(t, err)
	require.Equalf(t, 5, cfg.RateLimits.APIPerMinute, "APIPerMinute = %d, want explicit 5", cfg.RateLimits.APIPerMinute)
}

func TestConfigLoadE2EProfileIgnoresMetricsAddr(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_METRICS_ADDR", "127.0.0.1:9090")
	cfg, err := config.Load("e2e")
	require.NoError(t, err)
	require.Emptyf(t, cfg.MetricsAddr, "e2e MetricsAddr = %q, want empty so runs never collide on a port", cfg.MetricsAddr)
	dev, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, "127.0.0.1:9090", dev.MetricsAddr, "test profile MetricsAddr = %q, want the configured one", dev.MetricsAddr)
}

type internalAddrCase struct {
	name    string
	port    string
	metrics string
	addr    string
}

func loadWithInternalAddr(t *testing.T, tc internalAddrCase) (*config.Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", tc.port)
	t.Setenv("SPHERICON_METRICS_ADDR", tc.metrics)
	t.Setenv("SPHERICON_INTERNAL_ADDR", tc.addr)
	return config.Load("test")
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
	cfg, err := config.Load("e2e")
	require.NoError(t, err)
	require.Emptyf(t, cfg.InternalAddr, "e2e InternalAddr = %q, want empty so runs never collide on a port", cfg.InternalAddr)
}

func TestConfigLoadSMTPAllowPrivateHostsFollowsTheEnvironmentUnlessSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.True(t, cfg.SMTPAllowPrivateHosts, "a non-production environment allows private SMTP hosts by default")

	t.Setenv("SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS", "false")
	cfg, err = config.Load("test")
	require.NoError(t, err)
	require.False(t, cfg.SMTPAllowPrivateHosts, "an explicit false must win over the environment default")
}

func TestConfigLoadAbuseLimitsDefaultToTheADR(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg, err := config.Load("test")
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
			_, err := config.Load("test")
			require.Error(t, err, "want an error")
		})
	}
}

func TestConfigLoadOutboundAllowIsEmptyByDefaultAndRejectsMalformedValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	_, err := config.Load("test")
	require.NoError(t, err)

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n:5678,node-red.lan:1880")
	_, err = config.Load("test")
	require.NoErrorf(t, err, "a list of host:port pairs must load: %v", err)

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n")
	_, err = config.Load("test")
	require.ErrorContains(t, err, "SPHERICON_OUTBOUND_ALLOW", "a malformed value must fail the load naming the variable")
}

// secretFile writes content to a temp file and returns its path.
func secretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// The e2e profile reads no env file (none is committed for it), so every secret left unset
// comes from its file.
func TestConfigLoadSecretFromFile(t *testing.T) {
	for _, name := range []string{
		"DATABASE_URL", "SPHERICON_JWT_SECRET", "SPHERICON_JWT_SECRET_SECONDARY", "SPHERICON_ENCRYPTION_KEY",
		"SPHERICON_SMTP_PASS", "SPHERICON_LICENSE_KEY",
	} {
		t.Setenv(name, "")
	}
	other := strings.Repeat("m3", 16)
	t.Setenv("DATABASE_URL_FILE", secretFile(t, "postgres://from-file\n"))
	t.Setenv("SPHERICON_JWT_SECRET_FILE", secretFile(t, strongSecret+"\r\n"))
	t.Setenv("SPHERICON_JWT_SECRET_SECONDARY_FILE", secretFile(t, other))
	t.Setenv("SPHERICON_ENCRYPTION_KEY_FILE", secretFile(t, "keyset"))
	t.Setenv("SPHERICON_SMTP_PASS_FILE", secretFile(t, "smtp-pass\n"))
	t.Setenv("SPHERICON_LICENSE_KEY_FILE", secretFile(t, "license.jwt\n"))

	cfg, err := config.Load("e2e")
	require.NoErrorf(t, err, "Load: %v", err)
	for name, got := range map[string][2]string{
		"DatabaseURL":        {cfg.DatabaseURL, "postgres://from-file"},
		"JWTSecret":          {cfg.JWTSecret, strongSecret},
		"JWTSecretSecondary": {cfg.JWTSecretSecondary, other},
		"EncryptionKey":      {cfg.EncryptionKey, "keyset"},
		"SMTPPass":           {cfg.SMTPPass, "smtp-pass"},
		"LicenseKey":         {cfg.LicenseKey, "license.jwt"},
	} {
		assert.Equalf(t, got[1], got[0], "%s = %q, want %q", name, got[0], got[1])
	}
}

func TestConfigLoadExplicitSecretWinsOverFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://explicit")
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "missing"))

	cfg, err := config.Load("test")
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
			t.Setenv("SPHERICON_LICENSE_KEY", "")
			t.Setenv("SPHERICON_LICENSE_KEY_FILE", path)

			_, err := config.Load("test")
			require.ErrorContains(t, err, "SPHERICON_LICENSE_KEY_FILE")
		})
	}
}

// loadWithSecondarySecret pins the mode, as loadWithJWTSecret does.
func loadWithSecondarySecret(t *testing.T, env, secondary string) (*config.Config, error) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_MODE", "selfhosted")
	t.Setenv("SPHERICON_JWT_SECRET", strongSecret)
	t.Setenv("SPHERICON_JWT_SECRET_SECONDARY", secondary)
	return config.Load(env)
}

func TestConfigLoadJWTSecretSecondaryIsVerifyOnlyAndOptional(t *testing.T) {
	other := strings.Repeat("m3", 16)
	for name, tc := range map[string]struct {
		env, secondary string
		wantVerify     []string
	}{
		"none":                   {env: "test", secondary: "", wantVerify: nil},
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

			require.ErrorContains(t, err, "SPHERICON_JWT_SECRET_SECONDARY")
		})
	}
}

func TestConfigLoadEventLiveWindow(t *testing.T) {
	cfg, err := config.Load("test")
	require.NoError(t, err)
	require.Equalf(t, 72*time.Hour, cfg.EventLiveWindow, "default EventLiveWindow = %v, want 72h", cfg.EventLiveWindow)

	t.Setenv("SPHERICON_EVENT_LIVE_WINDOW_HOURS", "0")
	_, err = config.Load("test")
	require.Error(t, err, "a live window under an hour must be rejected: it cannot be switched off")
}

var secretLooking = regexp.MustCompile(`SECRET|TOKEN|_PASS$|_KEY$|_KEY_ID$|^DATABASE_URL$`)

// secretVariables are the variables Load reads whose names look secret. Their companions are set,
// for a secret Load refuses alone: the account link's URL, the sign-in providers' client ids.
func secretVariables(t *testing.T) []string {
	t.Helper()
	t.Setenv("SPHERICON_ACCOUNT_LINK_URL", "https://account.example.com")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_ID", "github-client")
	secrets := lo.Filter(config.Variables(), func(name string, _ int) bool {
		return secretLooking.MatchString(name)
	})
	require.NotEmpty(t, secrets)
	return secrets
}

func TestEverySecretVariableIsRedacted(t *testing.T) {
	secrets := secretVariables(t)
	for _, name := range secrets {
		t.Setenv(name, "value-of-"+name)
	}

	cfg, err := config.Load("test")
	require.NoError(t, err)
	values := config.SecretValues(cfg)
	for _, name := range secrets {
		require.Containsf(t, values, "value-of-"+name, "%s is not redacted", name)
	}
}

func TestEverySecretVariableAcceptsAFile(t *testing.T) {
	// .env.test sets the encryption key, and a value set directly wins over its file.
	secrets := lo.Without(secretVariables(t), "SPHERICON_ENCRYPTION_KEY")
	dir := t.TempDir()
	for _, name := range secrets {
		t.Setenv(name, "")
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("file-of-"+name+"\n"), 0o600))
		t.Setenv(name+"_FILE", path)
	}

	cfg, err := config.Load("test")
	require.NoError(t, err)
	values := config.SecretValues(cfg)
	for _, name := range secrets {
		require.Containsf(t, values, "file-of-"+name, "%s_FILE is not read", name)
	}
}

func TestConfigLoadRefusesAValueThatBreaksItsRule(t *testing.T) {
	for env, want := range map[string]string{
		"SPHERICON_SESSION_TTL=0s":                                     "SPHERICON_SESSION_TTL must be positive",
		"SPHERICON_STAFF_SESSION_TTL=0s":                               "SPHERICON_STAFF_SESSION_TTL must be positive",
		"SPHERICON_STAFF_HOLD_MAX_EXPIRY=0s":                           "SPHERICON_STAFF_HOLD_MAX_EXPIRY must be positive",
		"SPHERICON_ABUSE_VOLUME_FLOOR=0":                               "SPHERICON_ABUSE_VOLUME_FLOOR must be positive",
		"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT=0":                     "SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT must be positive",
		"SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT=0.5":                   "SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT must be below SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT",
		"SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT=101":                 "SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT must be at most 100",
		"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT=0":                        "SPHERICON_ABUSE_WARN_BOUNCE_PERCENT must be positive",
		"SPHERICON_ABUSE_WARN_BOUNCE_PERCENT=20":                       "SPHERICON_ABUSE_WARN_BOUNCE_PERCENT must be below SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT",
		"SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT=101":                    "SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT must be at most 100",
		"SPHERICON_ACCOUNT_LINK_URL=https://account.example.com":       "SPHERICON_ACCOUNT_LINK_SIGNING_KEY must be set together with SPHERICON_ACCOUNT_LINK_URL",
		"SPHERICON_ACCOUNT_LINK_SIGNING_KEY=key":                       "SPHERICON_ACCOUNT_LINK_URL must be set together with SPHERICON_ACCOUNT_LINK_SIGNING_KEY",
		"SPHERICON_MAX_BODY_BYTES=0":                                   "SPHERICON_MAX_BODY_BYTES must be positive",
		"SPHERICON_COLLECT_MAX_BODY_BYTES=0":                           "SPHERICON_COLLECT_MAX_BODY_BYTES must be positive",
		"SPHERICON_COLLECT_MAX_EVENT_BYTES=0":                          "SPHERICON_COLLECT_MAX_EVENT_BYTES must be positive",
		"SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE=-1":                     "SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND=-1":                 "SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND must not be negative",
		"SPHERICON_RATE_LIMIT_API_PER_MINUTE=-1":                       "SPHERICON_RATE_LIMIT_API_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE=-1":               "SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE=-1":                  "SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_LOGIN_FAILURES=-1":                       "SPHERICON_RATE_LIMIT_LOGIN_FAILURES must not be negative",
		"SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE=-1":                  "SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE=-1":                   "SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE=-1":                "SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE=-1":                   "SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE must not be negative",
		"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR=-1": "SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR must not be negative",
		"SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR=-1":          "SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR must not be negative",
		"SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS=-1":                     "SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS must not be negative",
		"SPHERICON_EVENTS_RETENTION_DAYS=-1":                           "SPHERICON_EVENTS_RETENTION_DAYS must not be negative",
		"SPHERICON_UNVERIFIED_USER_RETENTION_DAYS=-1":                  "SPHERICON_UNVERIFIED_USER_RETENTION_DAYS must not be negative",
		"SPHERICON_EVENT_LIVE_WINDOW_HOURS=0":                          "SPHERICON_EVENT_LIVE_WINDOW_HOURS must be at least 1",
		"SPHERICON_TEST_INBOX_RETENTION_DAYS=0":                        "SPHERICON_TEST_INBOX_RETENTION_DAYS must be at least 1",
		"SPHERICON_TEST_INBOX_MAX_MESSAGES=0":                          "SPHERICON_TEST_INBOX_MAX_MESSAGES must be at least 1",
		"SPHERICON_DB_MAX_OPEN_CONNS=0":                                "SPHERICON_DB_MAX_OPEN_CONNS must be positive",
		"SPHERICON_DB_MAX_IDLE_CONNS=-1":                               "SPHERICON_DB_MAX_IDLE_CONNS must not be negative",
		"SPHERICON_DB_MAX_IDLE_CONNS=100":                              "SPHERICON_DB_MAX_IDLE_CONNS must not be above SPHERICON_DB_MAX_OPEN_CONNS",
		"SPHERICON_DB_CONN_MAX_LIFETIME=0s":                            "SPHERICON_DB_CONN_MAX_LIFETIME must be positive",
		"SPHERICON_RIVER_DB_MAX_CONNS=0":                               "SPHERICON_RIVER_DB_MAX_CONNS must be positive",
	} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			name, value, _ := strings.Cut(env, "=")
			t.Setenv(name, value)
			_, err := config.Load("test")
			require.ErrorContains(t, err, want)
		})
	}
}

func TestConfigLoadProfileDefaults(t *testing.T) {
	for profile, want := range map[string]struct {
		limits       config.RateLimits
		logFormat    string
		privateHosts bool
	}{
		"":            {config.DefaultRateLimits, "text", true},
		"development": {config.DefaultRateLimits, "text", true},
		"test":        {config.DefaultRateLimits, "text", true},
		"e2e":         {config.RateLimits{}, "json", true},
		"production":  {config.DefaultRateLimits, "json", false},
	} {
		t.Run(profile, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("SPHERICON_JWT_SECRET", strongSecret)
			cfg, err := config.Load(profile)
			require.NoError(t, err)
			assert.Equal(t, want.limits, cfg.RateLimits)
			assert.Equal(t, want.logFormat, cfg.LogFormat)
			assert.Equal(t, want.privateHosts, cfg.SMTPAllowPrivateHosts)
		})
	}
}

func TestConfigLoadAVariableReplacesItsProfileDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE", "7")
	t.Setenv("SPHERICON_LOG_FORMAT", "json")
	t.Setenv("SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS", "false")

	cfg, err := config.Load("e2e")
	require.NoError(t, err)
	assert.Equal(t, config.RateLimits{LoginIP: 7}, cfg.RateLimits)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.False(t, cfg.SMTPAllowPrivateHosts)
}
