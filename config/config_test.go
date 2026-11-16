package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// unset removes a variable for the test and restores it afterwards.
func unset(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	require.NoError(t, os.Unsetenv(name))
}

func TestConfigLoadRefusesToStartWithoutTheRequiredSecrets(t *testing.T) {
	for _, name := range []string{"SPHERICON_DATABASE_URL", "SPHERICON_JWT_SECRET", "SPHERICON_ENCRYPTION_KEY"} {
		t.Run(name+" unset", func(t *testing.T) {
			unset(t, name)
			_, err := config.Load(config.EnvTest)
			require.ErrorContains(t, err, name)
		})
		t.Run(name+" empty", func(t *testing.T) {
			t.Setenv(name, "")
			_, err := config.Load(config.EnvTest)
			require.ErrorContains(t, err, name)
		})
	}
}

func TestConfigLoadModeTypoStopsTheStart(t *testing.T) {
	t.Setenv("SPHERICON_MODE", "saass")
	_, err := config.Load(config.EnvTest)
	require.ErrorContains(t, err, "SPHERICON_MODE")

	t.Setenv("SPHERICON_MODE", "SaaS")
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, config.ModeSaaS, cfg.Mode)
}

func TestConfigLoadModeDecidesWhatDiffers(t *testing.T) {
	unset(t, "SPHERICON_MODE")

	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, config.ModeSelfHosted, cfg.Mode)
	require.False(t, cfg.RequireEmailVerification, "selfhosted does not require email verification")
	require.False(t, cfg.SignupSurvey, "selfhosted has no Sign-up survey")
	require.True(t, cfg.ReleaseCheck, "selfhosted checks for a release")
	require.Nil(t, cfg.Telemetry, "selfhosted leaves the Telemetry report to the Owner")

	cfg.ApplyMode(config.ModeSaaS)
	require.True(t, cfg.RequireEmailVerification, "saas requires email verification")
	require.True(t, cfg.SignupSurvey, "saas asks the Sign-up survey")
	require.False(t, cfg.ReleaseCheck, "saas makes no Release check")
	require.NotNil(t, cfg.Telemetry)
	require.False(t, *cfg.Telemetry, "saas keeps the Telemetry report off")
}

func TestConfigLoadEnvironmentSetsOnlyTheDevFlags(t *testing.T) {
	cases := map[string][2]bool{
		config.EnvDevelopment: {true, false},
		config.EnvTest:        {true, false},
		config.EnvE2E:         {false, true},
		config.EnvProduction:  {false, false},
		"":                    {false, false},
	}
	for envName, want := range cases {
		cfg, err := config.Load(envName)
		require.NoError(t, err)
		require.Equal(t, want, [2]bool{cfg.IsDev, cfg.IsE2E}, "SPHERICON_ENV=%q", envName)
	}
}

func TestConfigLoadIdleConnectionsFollowOpenUnlessSet(t *testing.T) {
	t.Setenv("SPHERICON_DB_MAX_OPEN_CONNS", "40")
	unset(t, "SPHERICON_DB_MAX_IDLE_CONNS")
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, 40, cfg.DBPool.MaxIdleConns)

	t.Setenv("SPHERICON_DB_MAX_IDLE_CONNS", "5")
	cfg, err = config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, 5, cfg.DBPool.MaxIdleConns)
}

func TestConfigLoadRefusesAnUnknownValueOfOurTypes(t *testing.T) {
	cases := map[string]string{
		"SPHERICON_LOG_FORMAT":     "xml",
		"SPHERICON_OUTBOUND_ALLOW": "n8n",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			_, err := config.Load(config.EnvTest)
			require.Error(t, err, "%s=%q must stop the start", name, value)
		})
	}
}

func TestConfigLoadOutboundAllowIsEmptyUnlessSet(t *testing.T) {
	unset(t, "SPHERICON_OUTBOUND_ALLOW")
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, webhook.Allowlist{}, cfg.OutboundAllow)

	t.Setenv("SPHERICON_OUTBOUND_ALLOW", "n8n:5678")
	cfg, err = config.Load(config.EnvTest)
	require.NoError(t, err)
	want, err := webhook.ParseAllowlist("n8n:5678")
	require.NoError(t, err)
	require.Equal(t, want, cfg.OutboundAllow)
}

func TestConfigLoadSigninProviderIsEnabledByItsClientID(t *testing.T) {
	unset(t, "SPHERICON_SIGNIN_GOOGLE_CLIENT_ID")
	unset(t, "SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_ID", "gh-id")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET", "gh-secret")

	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	require.Equal(t, map[string]config.SigninCredentials{
		"github": {ClientID: "gh-id", ClientSecret: "gh-secret"},
	}, cfg.Signin.Enabled())
}

func TestConfigTrackerOrigin(t *testing.T) {
	cfg := config.Config{AppURL: "https://app.example"}
	onAppOrigin := cfg.TrackerOrigin()
	cfg.TrackerURL = "https://t.example/"
	onOwnHost := cfg.TrackerOrigin()

	require.Equal(t, "https://app.example", onAppOrigin)
	require.Equal(t, "https://t.example", onOwnHost)
}

// secretLooking matches the names of the variables that hold a secret.
var secretLooking = regexp.MustCompile(`SECRET|TOKEN|_PASS$|_KEY$|_KEY_ID$|^SPHERICON_DATABASE_URL$`)

func TestEverySecretVariableIsRedacted(t *testing.T) {
	secrets := lo.Filter(config.Variables(), func(name string, _ int) bool { return secretLooking.MatchString(name) })
	require.NotEmpty(t, secrets)
	for _, name := range secrets {
		t.Setenv(name, "value-of-"+name)
	}

	values := config.SecretValues()
	for _, name := range secrets {
		require.Containsf(t, values, "value-of-"+name, "%s is not redacted", name)
	}
}

func TestSecretValuesRedactTheDatabasePassword(t *testing.T) {
	t.Setenv("SPHERICON_DATABASE_URL", "postgres://u:p%40ss@db/x")

	values := config.SecretValues()
	require.Contains(t, values, "p@ss")
}

// TestEveryVariableIsInTheReference keeps docs/configuration.md, which envdoc writes, in step
// with the env tags: a variable missing from it means the generation was not rerun.
func TestEveryVariableIsInTheReference(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "docs", "configuration.md"))
	require.NoError(t, err)
	reference := string(raw)
	for _, name := range config.Variables() {
		require.Contains(t, reference, "`"+name+"`", "%s is missing from docs/configuration.md: run `mise run generate:configdoc`", name)
	}
}
