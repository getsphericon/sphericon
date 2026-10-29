package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

func TestConfigLoadSigninProviderIsEnabledByBothVariables(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET", "client-secret")

	cfg, err := config.Load("test")

	require.NoError(t, err)
	require.Equal(t, config.SigninCredentials{ClientID: "client-id", ClientSecret: "client-secret"}, cfg.Signin["google"])
}

func TestConfigLoadSigninEachProviderReadsItsOwnVariables(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_ID", "gh-id")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET", "gh-secret")

	cfg, err := config.Load("test")

	require.NoError(t, err)
	require.Equal(t, config.SigninCredentials{ClientID: "gh-id", ClientSecret: "gh-secret"}, cfg.Signin["github"])
}

func TestConfigLoadSigninGitHubWithoutItsSecretStopsTheStartAndNamesIt(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_ID", "gh-id")
	// An empty value counts as unset, so a developer's .env would fill it in: a blank one is
	// trimmed to empty by the provider check and still wins over the file.
	t.Setenv("SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET", " ")

	_, err := config.Load("test")

	require.ErrorContains(t, err, "SPHERICON_SIGNIN_GITHUB_CLIENT_SECRET")
}

func TestConfigLoadSigninProviderIsOffWithNeitherVariable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", " ")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET", " ")

	cfg, err := config.Load("test")

	require.NoError(t, err)
	require.NotContains(t, cfg.Signin, "google")
}

func TestConfigLoadSigninProviderWithoutItsSecretStopsTheStartAndNamesIt(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET", " ")

	_, err := config.Load("test")

	require.ErrorContains(t, err, "SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET")
}

func TestConfigLoadSigninProviderWithoutItsIDStopsTheStartAndNamesIt(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_ID", " ")
	t.Setenv("SPHERICON_SIGNIN_GOOGLE_CLIENT_SECRET", "client-secret")

	_, err := config.Load("test")

	require.ErrorContains(t, err, "SPHERICON_SIGNIN_GOOGLE_CLIENT_ID")
}
