package signin_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/signin"
)

// Whatever arrives as `return_to`, what comes back is either nothing or a path inside the
// app: the flow can never send a person to another host.
func TestSafeReturnToIsEmptyOrARelativeInAppPath(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "raw")

		got := signin.SafeReturnTo(raw)

		require.True(t, got == "" || got == raw, "%q becomes %q: a safe value is returned as it came", raw, got)
		u, err := url.Parse(got)
		require.NoError(t, err)
		require.Empty(t, u.Scheme, "%q keeps a scheme", got)
		require.Empty(t, u.Host, "%q keeps a host", got)
		require.False(t, got != "" && !strings.HasPrefix(got, "/"), "%q is not rooted", got)
		require.False(t, strings.HasPrefix(got, "//"), "%q is protocol-relative", got)
		require.NotContains(t, got, `\`)
	})
}

func TestSafeReturnToKeepsAnInAppPathWithItsQuery(t *testing.T) {
	assert.Equal(t, "/w/abc/contacts?tab=1", signin.SafeReturnTo("/w/abc/contacts?tab=1"))
	assert.Equal(t, "/", signin.SafeReturnTo("/"))
}

func TestSafeReturnToDropsForeignTargets(t *testing.T) {
	for _, raw := range []string{
		"https://evil.test/", "http://evil.test", "//evil.test", "///evil.test", `/\evil.test`, `\\evil.test`,
		"evil.test", "javascript:alert(1)", "/ok\nSet-Cookie: x=1", "", "/path\t",
	} {
		assert.Empty(t, signin.SafeReturnTo(raw), "%q", raw)
	}
}

func FuzzSafeReturnTo(f *testing.F) {
	for _, seed := range []string{"/", "/a?b=c", "//x", "https://x", `/\x`, "", "/%2F/x", "/%5Cx"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got := signin.SafeReturnTo(raw)

		require.True(t, got == "" || got == raw)
		u, err := url.Parse(got)
		require.NoError(t, err)
		require.Empty(t, u.Host)
		require.Empty(t, u.Scheme)
	})
}

// Google vouches for an address only where it owns the mailbox: Gmail, or a hosted domain.
func TestGoogleTrustAcceptsAVerifiedGmailAddress(t *testing.T) {
	p := newProviders()

	got := p.Trust("google", signin.Profile{Email: "ann@gmail.com", EmailVerified: true})

	assert.Empty(t, got)
}

func TestGoogleTrustAcceptsAVerifiedAddressOfAHostedDomain(t *testing.T) {
	p := newProviders()

	got := p.Trust("google", signin.Profile{Email: "ann@corp.test", EmailVerified: true, HostedDomain: "corp.test"})

	assert.Empty(t, got)
}

func TestGoogleTrustRefusesAnotherAddressWithoutAHostedDomain(t *testing.T) {
	p := newProviders()

	got := p.Trust("google", signin.Profile{Email: "ann@corp.test", EmailVerified: true})

	assert.Equal(t, signin.ReasonEmailNotTrusted, got)
}

func TestGoogleTrustRefusesAnUnverifiedAddressWhateverItsDomain(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := newProviders()
		email := rapid.SampledFrom([]string{"ann@gmail.com", "ann@corp.test", "", "ANN@GMAIL.COM"}).Draw(t, "email")
		hd := rapid.SampledFrom([]string{"", "corp.test"}).Draw(t, "hd")

		got := p.Trust("google", signin.Profile{Email: email, EmailVerified: false, HostedDomain: hd})

		require.Equal(t, signin.ReasonEmailUnverified, got)
	})
}

func TestTrustRefusesAProviderThatIsNotEnabled(t *testing.T) {
	p := signin.New(nil, "https://app.test")

	got := p.Trust("google", signin.Profile{Email: "ann@gmail.com", EmailVerified: true})

	assert.Equal(t, signin.ReasonProviderUnavailable, got)
}

func TestOnlyConfiguredProvidersAreListed(t *testing.T) {
	none := signin.New(nil, "https://app.test")
	some := newProviders()

	assert.Empty(t, none.Enabled())
	assert.Equal(t, []signin.Info{{Name: "google", DisplayName: "Google"}}, some.Enabled())
}

func TestRedirectURLDerivesFromTheAppURL(t *testing.T) {
	assert.Equal(t, "https://app.test/site/auth/signin/google/callback", signin.RedirectURL("https://app.test", "google"))
	assert.Equal(t, "https://app.test/site/auth/signin/google/callback", signin.RedirectURL("https://app.test/", "google"))
}

func TestStartDrawsAFreshFlowAndSendsPKCEAndTheRedirectURI(t *testing.T) {
	p := newProviders()

	authURL, flow, ok := p.Start("google", "/w/x")
	_, other, _ := p.Start("google", "/w/x")

	require.True(t, ok)
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "client-id", q.Get("client_id"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))
	assert.Equal(t, flow.State, q.Get("state"))
	assert.Equal(t, flow.Nonce, q.Get("nonce"))
	assert.Equal(t, "https://app.test/site/auth/signin/google/callback", q.Get("redirect_uri"))
	assert.NotEqual(t, flow.State, other.State, "every flow draws its own state")
	assert.NotEqual(t, flow.Verifier, other.Verifier)
}

func TestStartRefusesAProviderThatIsNotEnabled(t *testing.T) {
	p := newProviders()

	_, _, ok := p.Start("github", "")

	assert.False(t, ok)
}

// A flow opens only as it was sealed, for its own provider, while it lives.
func TestFlowRoundTripsThroughTheCookie(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		codec := signin.NewFlowCodec("secret-0123456789", nil)
		flow := signin.Flow{
			Provider: "google", State: rapid.StringMatching(`[a-z0-9]{1,40}`).Draw(t, "state"),
			Verifier: rapid.String().Draw(t, "verifier"), Nonce: rapid.String().Draw(t, "nonce"), ReturnTo: rapid.String().Draw(t, "return"),
		}

		sealed, err := codec.Seal(flow, time.Minute)
		require.NoError(t, err)
		opened, err := codec.Open(sealed, "google")

		require.NoError(t, err)
		require.Equal(t, flow, opened)
	})
}

func TestFlowDoesNotOpenForAnotherProvider(t *testing.T) {
	codec := signin.NewFlowCodec("secret-0123456789", nil)
	sealed, err := codec.Seal(signin.Flow{Provider: "google", State: "s"}, time.Minute)
	require.NoError(t, err)

	_, err = codec.Open(sealed, "github")

	assert.ErrorIs(t, err, signin.ErrFlow)
}

func TestFlowDoesNotOpenUnderAnotherSecret(t *testing.T) {
	sealed, err := signin.NewFlowCodec("secret-0123456789", nil).Seal(signin.Flow{Provider: "google", State: "s"}, time.Minute)
	require.NoError(t, err)

	_, err = signin.NewFlowCodec("another-secret-987654", nil).Open(sealed, "google")

	assert.ErrorIs(t, err, signin.ErrFlow)
}

func TestFlowDoesNotOpenOnceItExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	sealed, err := signin.NewFlowCodec("secret-0123456789", func() time.Time { return now }).
		Seal(signin.Flow{Provider: "google", State: "s"}, time.Minute)
	require.NoError(t, err)

	_, err = signin.NewFlowCodec("secret-0123456789", func() time.Time { return now.Add(2 * time.Minute) }).Open(sealed, "google")

	assert.ErrorIs(t, err, signin.ErrFlow)
}

func TestFlowStateMatchesOnlyTheSameNonEmptyState(t *testing.T) {
	flow := signin.Flow{State: "abc"}

	assert.True(t, flow.StateMatches("abc"))
	assert.False(t, flow.StateMatches("xyz"))
	assert.False(t, flow.StateMatches(""))
	assert.False(t, signin.Flow{}.StateMatches(""))
}

// Whatever a browser sends as the flow cookie is a flow or the one refusal, never a panic.
func FuzzOpenFlow(f *testing.F) {
	codec := signin.NewFlowCodec("secret-0123456789", nil)
	sealed, err := codec.Seal(signin.Flow{Provider: "google", State: "s", ReturnTo: "/x"}, time.Minute)
	require.NoError(f, err)
	f.Add(sealed)
	f.Add("")
	f.Add("a.b.c")
	f.Fuzz(func(t *testing.T, raw string) {
		flow, err := codec.Open(raw, "google")

		require.True(t, err == nil || errors.Is(err, signin.ErrFlow), "unexpected error %v", err)
		require.True(t, err != nil || flow.State != "", "a flow with no state opened")
	})
}

func newProviders() *signin.Providers {
	return signin.New(map[string]config.SigninCredentials{"google": {ClientID: "client-id", ClientSecret: "client-secret"}}, "https://app.test")
}

func TestReasonsAreTheContractsErrorCodes(t *testing.T) {
	got := make([]string, 0, len(signin.Reasons()))
	for _, r := range signin.Reasons() {
		got = append(got, string(r))
	}

	assert.ElementsMatch(t, contractSigninErrors(t), got)
}
