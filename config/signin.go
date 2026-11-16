package config

// SigninProviders are the OAuth clients of the Sign-in providers (ADR 0048), one per provider the
// Instance can enable. A provider is enabled by its client id.
type SigninProviders struct {
	// Google enables "Continue with Google". Register
	// `<SPHERICON_APP_URL>/site/auth/signin/google/callback` as the redirect URI of the Instance's
	// Google Cloud app.
	Google SigninCredentials `envPrefix:"SPHERICON_SIGNIN_GOOGLE_"`
	// GitHub enables "Continue with GitHub". Set `<SPHERICON_APP_URL>/site/auth/signin/github/callback`
	// as the callback URL of the Instance's GitHub OAuth app.
	GitHub SigninCredentials `envPrefix:"SPHERICON_SIGNIN_GITHUB_"`
}

// SigninCredentials is the OAuth client of one Sign-in provider: the Instance operator registers
// an app with the provider and sets its id and secret.
type SigninCredentials struct {
	// The OAuth client id of the provider's app; set, it enables the provider.
	ClientID string `env:"CLIENT_ID"`
	// The OAuth client secret of the provider's app.
	ClientSecret string `env:"CLIENT_SECRET"`
}

// Enabled are the providers with a client id, by registry name.
func (p SigninProviders) Enabled() map[string]SigninCredentials {
	enabled := map[string]SigninCredentials{}
	for name, c := range map[string]SigninCredentials{"google": p.Google, "github": p.GitHub} {
		if c.ClientID != "" {
			enabled[name] = c
		}
	}
	return enabled
}
