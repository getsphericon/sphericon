package doctor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/doctor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// stubbed are the seams with every reach outside the process stubbed: the relay answers smtpErr,
// the Instance answers /healthz, and no DKIM record is published.
func stubbed(smtpErr error) doctor.Seams {
	return doctor.Seams{
		Now:      func() time.Time { return now },
		DialSMTP: func(context.Context, *config.Config) error { return smtpErr },
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})},
		LookupTXT: func(_ context.Context, name string) ([]string, error) {
			return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		},
	}
}

func TestDiagnoseFailsAndPrintsTheReportInEitherFormat(t *testing.T) {
	key, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", testhelper.RealDBURL(t))
	t.Setenv("SPHERICON_ENCRYPTION_KEY", key)
	t.Setenv("SPHERICON_JWT_SECRET", "0123456789abcdef0123456789abcdef-not-a-placeholder")
	t.Setenv("SPHERICON_APP_URL", "https://sphericon.example.com")
	t.Setenv("SPHERICON_SMTP_HOST", "smtp.example.test")

	var text bytes.Buffer
	err = doctor.Diagnose(t.Context(), "test", stubbed(nil), &text, false)
	require.ErrorIs(t, err, doctor.ErrChecksFailed, "the fixture Sending domains publish no DKIM record")
	assert.Contains(t, text.String(), "[ ok ] database")
	assert.Contains(t, text.String(), "[FAIL] dkim ")
	assert.Contains(t, text.String(), "fix: ")

	var js bytes.Buffer
	err = doctor.Diagnose(t.Context(), "test", stubbed(nil), &js, true)
	require.ErrorIs(t, err, doctor.ErrChecksFailed)
	var report struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(js.Bytes(), &report), "the output is the JSON report and nothing else")
	assert.False(t, report.OK)
	assert.NotEmpty(t, report.Checks)
}

func TestDiagnoseNeverPrintsASecret(t *testing.T) {
	secretValues := map[string]string{
		"DATABASE_URL":             "postgres://doctor:DbPasswordSentinel9@127.0.0.1:1/nowhere?sslmode=disable",
		"SPHERICON_JWT_SECRET":     "JwtSecretSentinel-change-me-0123456789",
		"SPHERICON_ENCRYPTION_KEY": "EncryptionKeySentinel!!",
		"SPHERICON_SMTP_PASS":      "SmtpPasswordSentinel7",
		"SPHERICON_LICENSE_KEY":    "LicenseKeySentinel.not.valid",
	}
	for name, value := range secretValues {
		t.Setenv(name, value)
	}
	t.Setenv("SPHERICON_SMTP_HOST", "smtp.example.test")
	t.Setenv("SPHERICON_SMTP_USER", "mailer")
	// A relay error that echoes the credentials, the way a careless server or library might.
	echo := errors.New("535 rejected user mailer password SmtpPasswordSentinel7 via " + secretValues["DATABASE_URL"])

	for _, asJSON := range []bool{false, true} {
		var out bytes.Buffer
		err := doctor.Diagnose(t.Context(), "test", stubbed(echo), &out, asJSON)
		require.ErrorIs(t, err, doctor.ErrChecksFailed)

		assert.Contains(t, out.String(), "[redacted]", "the error text reached the report, so the redactor was exercised")
		for name, value := range secretValues {
			assert.NotContains(t, out.String(), value, "%s, json %v", name, asJSON)
		}
		assert.NotContains(t, out.String(), "DbPasswordSentinel9")
		assert.NotContains(t, out.String(), "SmtpPasswordSentinel7")
	}
}
