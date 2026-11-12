package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func TestGenerateSiteRateLimitsRoutesEachLimitedOperationUnderTheSurfacePrefix(t *testing.T) {
	src, err := scopegen.SiteRateLimits.Generate([]byte(`{"paths":{
	 "/auth/login":{"post":{"operationId":"SiteAuth_login","x-rate-limit":"login"}},
	 "/auth/forgot-password":{"post":{"operationId":"SiteAuth_forgotPassword","x-rate-limit":"forgot_password"}},
	 "/invitations/{token}/accept":{"post":{"operationId":"SitePublicInvitations_accept","x-rate-limit":"human"}},
	 "/me":{"get":{"operationId":"SiteUser_getMe"}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package ratelimit")
	require.Regexp(t, `"POST " \+ paths\.Site \+ "/auth/login":\s+RouteLimitLogin,`, out)
	require.Regexp(t, `"POST " \+ paths\.Site \+ "/auth/forgot-password":\s+RouteLimitForgotPassword,`, out)
	require.Regexp(t, `"POST " \+ paths\.Site \+ "/invitations/\{token\}/accept":\s+RouteLimitHuman,`, out)
	require.NotContains(t, out, "/me")
}

func TestGenerateRateLimitsRefusesAPolicyThatIsNotAString(t *testing.T) {
	_, err := scopegen.StaffRateLimits.Generate([]byte(`{"paths":{
	 "/auth/login":{"post":{"operationId":"StaffAuth_login","x-rate-limit":true}}}}`))

	require.ErrorContains(t, err, "x-rate-limit")
}
