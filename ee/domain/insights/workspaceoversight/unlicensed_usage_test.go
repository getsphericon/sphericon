package workspaceoversight_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestWithoutALicenseSendingAndContactChangesWriteNoUsageRows(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())
	fixtureRows := h.env.DB.Usage.Query().CountX(t.Context())
	// test writes: the contacts-peak hook fires on this Contact write
	require.NoError(t, h.env.DB.Scoped(fixtures.AcmeID).Contact().Create().SetEmail("new@example.com").Exec(t.Context()))
	h.sendOnAPI(t, "unlicensed@example.com")

	require.NoError(t, usages.Refresh(t.Context(), h.env.DB, nil, time.Now()))

	assert.Equal(t, fixtureRows, h.env.DB.Usage.Query().CountX(t.Context()), "no snapshot, counter or live row is written")
}

func TestWithoutALicenseTheServiceEndpointsAnswer404OnThePublicPort(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	for _, path := range []string{"/service/usage", "/service/workspaces/1/holds"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+primary)
		env.Server.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}

func TestSendOutcomesMatchWithAndWithoutALicenseUntilAHoldExists(t *testing.T) {
	licensed := newHarness(t)
	plain := newHarness(t, testhelper.WithoutLicense())

	licensed.sendOnAPI(t, "a@example.com")
	plain.sendOnAPI(t, "a@example.com")

	licensedMail := licensed.env.CustomerMail.Messages()
	plainMail := plain.env.CustomerMail.Messages()
	assert.Len(t, plainMail, len(licensedMail))
}
