package commandline_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/commandline"
)

var commandNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

func TestReportCredentialIssuedShowsTheValueOnceWithItsScopesAndExpiry(t *testing.T) {
	expires := commandNow.Add(24 * time.Hour)
	issued := &servicecredentials.Issued{
		ID: 7, Name: "svc-new", Value: "smsc_value",
		Scopes:    []servicecredentials.Scope{servicecredentials.ScopeUsageRead, servicecredentials.ScopeHoldsWrite},
		ExpiresAt: &expires,
	}
	var out bytes.Buffer

	err := commandline.ReportCredentialIssued(&out, issued, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "scopes: usage:read,holds:write")
	assert.Contains(t, out.String(), "expires: 2026-03-02T12:00:00Z")
	assert.Contains(t, out.String(), "value (shown once, only a hash is stored): smsc_value")
}

func TestReportCredentialIssuedExplainsARefusal(t *testing.T) {
	scope := commandline.ReportCredentialIssued(&bytes.Buffer{}, nil, servicecredentials.ErrInvalidScope)
	name := commandline.ReportCredentialIssued(&bytes.Buffer{}, nil, servicecredentials.ErrInvalidCredential)
	unlicensed := commandline.ReportCredentialIssued(&bytes.Buffer{}, nil, servicecredentials.ErrNotLicensed)

	require.ErrorContains(t, scope, "known scopes")
	require.ErrorContains(t, name, "an expiry in the future")
	require.ErrorContains(t, unlicensed, "no staff license")
}

func TestReportCredentialRotatedSaysTheOldCredentialStaysActiveUntilRevoked(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportCredentialRotated(&out, 7, &servicecredentials.Issued{ID: 8, Name: "svc-new", Value: "smsc_value"}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "smsc_value")
	assert.Contains(t, out.String(), "`sphericon service-credential revoke 7`")
}

func TestReportCredentialRotatedOrRevokedNamesTheCredentialItRefused(t *testing.T) {
	inactive := commandline.ReportCredentialRotated(&bytes.Buffer{}, 7, nil, servicecredentials.ErrCredentialInactive)
	missing := commandline.ReportCredentialRevoked(&bytes.Buffer{}, 999999, servicecredentials.ErrCredentialNotFound)

	require.EqualError(t, inactive, "service credential 7 is revoked or expired; nothing changed")
	require.EqualError(t, missing, "no service credential 999999; nothing changed")
}

func TestReportCredentialRevokedSaysTheCredentialIsRefusedFromNowOn(t *testing.T) {
	var out bytes.Buffer

	err := commandline.ReportCredentialRevoked(&out, 7, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "service credential 7 revoked")
}

func TestReportCredentialsShowsEachStateExpiryAndLastUsed(t *testing.T) {
	past, future := commandNow.Add(-time.Hour), commandNow.Add(time.Hour)
	list := []*ent.ServiceCredential{
		{ID: 1, Name: "svc-active", Scopes: []string{string(servicecredentials.ScopeUsageRead)}, ExpiresAt: &future, LastUsedAt: &past},
		{ID: 2, Name: "svc-revoked", Scopes: []string{string(servicecredentials.ScopeHoldsWrite)}, RevokedAt: &past},
		{ID: 3, Name: "svc-expired", Scopes: []string{string(servicecredentials.ScopeUsageRead)}, ExpiresAt: &commandNow},
	}
	var out bytes.Buffer

	err := commandline.ReportCredentials(&out, list, commandNow, nil)

	require.NoError(t, err)
	assert.Regexp(t, `1\s+svc-active\s+active\s+usage:read\s+2026-03-01T13:00:00Z\s+2026-03-01T11:00:00Z`, out.String())
	assert.Regexp(t, `2\s+svc-revoked\s+revoked\s+holds:write\s+never\s+never`, out.String())
	assert.Regexp(t, `3\s+svc-expired\s+expired`, out.String())
}
