package staff_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

var commandNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

func TestReportIssuedShowsTheValueOnceWithItsScopesAndExpiry(t *testing.T) {
	expires := commandNow.Add(24 * time.Hour)
	issued := &staff.Issued{
		ID: 7, Name: "svc-new", Value: "smsc_value",
		Scopes:    []serviceapi.ServiceScope{serviceapi.ServiceScopeUsageRead, serviceapi.ServiceScopeHoldsWrite},
		ExpiresAt: &expires,
	}
	var out bytes.Buffer

	err := staff.ReportIssued(&out, issued, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "scopes: usage:read,holds:write")
	assert.Contains(t, out.String(), "expires: 2026-03-02T12:00:00Z")
	assert.Contains(t, out.String(), "value (shown once, only a hash is stored): smsc_value")
}

func TestReportIssuedExplainsARefusal(t *testing.T) {
	scope := staff.ReportIssued(&bytes.Buffer{}, nil, staff.ErrInvalidScope)
	name := staff.ReportIssued(&bytes.Buffer{}, nil, staff.ErrInvalidCredential)
	unlicensed := staff.ReportIssued(&bytes.Buffer{}, nil, staff.ErrNotLicensed)

	require.ErrorContains(t, scope, "known scopes")
	require.ErrorContains(t, name, "a name and an expiry in the future")
	require.ErrorContains(t, unlicensed, "no staff license")
}

func TestReportRotatedSaysTheOldCredentialStaysActiveUntilRevoked(t *testing.T) {
	var out bytes.Buffer

	err := staff.ReportRotated(&out, 7, &staff.Issued{ID: 8, Name: "svc-new", Value: "smsc_value"}, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "smsc_value")
	assert.Contains(t, out.String(), "`sphericon service-credential revoke 7`")
}

func TestReportRotatedOrRevokedNamesTheCredentialItRefused(t *testing.T) {
	inactive := staff.ReportRotated(&bytes.Buffer{}, 7, nil, staff.ErrCredentialInactive)
	missing := staff.ReportRevoked(&bytes.Buffer{}, 999999, staff.ErrCredentialNotFound)

	require.EqualError(t, inactive, "service credential 7 is revoked or expired; nothing changed")
	require.EqualError(t, missing, "no service credential 999999; nothing changed")
}

func TestReportRevokedSaysTheCredentialIsRefusedFromNowOn(t *testing.T) {
	var out bytes.Buffer

	err := staff.ReportRevoked(&out, 7, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "service credential 7 revoked")
}

func TestReportCredentialsShowsEachStateExpiryAndLastUsed(t *testing.T) {
	past, future := commandNow.Add(-time.Hour), commandNow.Add(time.Hour)
	list := []*ent.ServiceCredential{
		{ID: 1, Name: "svc-active", Scopes: []string{string(serviceapi.ServiceScopeUsageRead)}, ExpiresAt: &future, LastUsedAt: &past},
		{ID: 2, Name: "svc-revoked", Scopes: []string{string(serviceapi.ServiceScopeHoldsWrite)}, RevokedAt: &past},
		{ID: 3, Name: "svc-expired", Scopes: []string{string(serviceapi.ServiceScopeUsageRead)}, ExpiresAt: &commandNow},
	}
	var out bytes.Buffer

	err := staff.ReportCredentials(&out, list, commandNow, nil)

	require.NoError(t, err)
	assert.Regexp(t, `1\s+svc-active\s+active\s+usage:read\s+2026-03-01T13:00:00Z\s+2026-03-01T11:00:00Z`, out.String())
	assert.Regexp(t, `2\s+svc-revoked\s+revoked\s+holds:write\s+never\s+never`, out.String())
	assert.Regexp(t, `3\s+svc-expired\s+expired`, out.String(), "a credential is expired from its expiry on")
}
