package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	forgotPath    = "/site/auth/forgot-password"
	forgotAddress = 3
	unknownEmail  = "nobody@example.com"
)

func forgot(t *testing.T, env *testhelper.TestEnv, email string) *httptest.ResponseRecorder {
	t.Helper()
	return postJSON(t, env, forgotPath, `{"email":"`+email+`"}`, nil)
}

func forgotEnv(t *testing.T, perAddress, perIP int) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithRateLimits(config.RateLimits{ForgotAddress: perAddress, ForgotIP: perIP}))
}

func TestForgotPasswordSendsNoMoreThanTheLimitPerAddressAndStaysSilent(t *testing.T) {
	env := forgotEnv(t, forgotAddress, 0)
	for i := range forgotAddress {
		rec := forgot(t, env, fixtures.OwnerJohnEmail)
		require.Equal(t, http.StatusAccepted, rec.Code, "request %d", i+1)
	}
	require.Len(t, env.SystemMail.Messages(), forgotAddress)

	rec := forgot(t, env, fixtures.OwnerJohnEmail)
	assert.Equal(t, http.StatusAccepted, rec.Code, "over the limit is still a silent 202")
	assert.Len(t, env.SystemMail.Messages(), forgotAddress, "nothing is sent over the limit")
}

// Parallel requests for one address must not slip a further mail past the budget.
func TestForgotPasswordSendsNoMoreThanTheLimitUnderConcurrency(t *testing.T) {
	env := forgotEnv(t, forgotAddress, 0)
	const requests = 20
	var wg sync.WaitGroup
	for range requests {
		wg.Go(func() {
			rec := forgot(t, env, fixtures.OwnerJohnEmail)
			assert.Equal(t, http.StatusAccepted, rec.Code)
		})
	}
	wg.Wait()
	assert.Len(t, env.SystemMail.Messages(), forgotAddress)
}

func TestForgotPasswordAddressLimitIsCaseInsensitive(t *testing.T) {
	env := forgotEnv(t, 1, 0)
	require.Equal(t, http.StatusAccepted, forgot(t, env, fixtures.OwnerJohnEmail).Code)
	require.Equal(t, http.StatusAccepted, forgot(t, env, strings.ToUpper(fixtures.OwnerJohnEmail)).Code)
	assert.Len(t, env.SystemMail.Messages(), 1, "the same address in another case shares the budget")
}

func TestForgotPasswordKnownAndUnknownEmailAnswerIdentically(t *testing.T) {
	env := forgotEnv(t, forgotAddress, 0)
	known := forgot(t, env, fixtures.OwnerJohnEmail)
	unknown := forgot(t, env, unknownEmail)
	assert.Equal(t, known.Code, unknown.Code)
	assert.Equal(t, known.Body.String(), unknown.Body.String())
	assert.Len(t, env.SystemMail.Messages(), 1, "only the known address gets mail")

	// An unknown address spends its budget exactly like a known one, so the
	// over-limit answer cannot tell them apart either.
	for range forgotAddress {
		forgot(t, env, unknownEmail)
	}
	over := forgot(t, env, unknownEmail)
	assert.Equal(t, known.Code, over.Code)
	assert.Equal(t, known.Body.String(), over.Body.String())
}

func TestForgotPasswordAnswers429OverTheLimitPerIP(t *testing.T) {
	const perIP = 4
	env := forgotEnv(t, 0, perIP)
	for i := range perIP {
		rec := forgot(t, env, fixtures.OwnerJohnEmail)
		require.Equal(t, http.StatusAccepted, rec.Code, "request %d", i+1)
	}
	rec := forgot(t, env, unknownEmail)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "3600", rec.Header().Get("Retry-After"))
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"retryAfter":3600`)
}

func TestForgotPasswordIsNotLimitedWhenDisabled(t *testing.T) {
	env := forgotEnv(t, 0, 0)
	for range 12 {
		require.Equal(t, http.StatusAccepted, forgot(t, env, fixtures.OwnerJohnEmail).Code)
	}
	assert.Len(t, env.SystemMail.Messages(), 12)
}
