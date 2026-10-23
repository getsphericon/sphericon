package main

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
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// doctorDeps is deps with every reach outside the process stubbed: the relay answers smtpErr,
// the Instance answers /healthz, and no DKIM record is published.
func doctorDeps(smtpErr error) deps {
	return deps{
		env: "test",
		now: func() time.Time { return cliNow },
		doctor: doctorSeams{
			dialSMTP: func(context.Context, *config.Config) error { return smtpErr },
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			})},
			lookupTXT: func(_ context.Context, name string) ([]string, error) {
				return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
			},
		},
	}
}

func TestDoctorExitsWithFailureAndPrintsTheReport(t *testing.T) {
	key, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", testhelper.RealDBURL(t))
	t.Setenv("SPHERICON_ENCRYPTION_KEY", key)
	t.Setenv("SPHERICON_JWT_SECRET", "0123456789abcdef0123456789abcdef-not-a-placeholder")
	t.Setenv("SPHERICON_APP_URL", "https://sphericon.example.com")
	t.Setenv("SPHERICON_SMTP_HOST", "smtp.example.test")

	var text bytes.Buffer
	err = runDeps(doctorDeps(nil), &text, "doctor")
	require.ErrorIs(t, err, errChecksFailed, "the fixture Sending domains publish no DKIM record")
	assert.Contains(t, text.String(), "[ ok ] database")
	assert.Contains(t, text.String(), "[FAIL] dkim ")
	assert.Contains(t, text.String(), "fix: ")

	var js bytes.Buffer
	require.ErrorIs(t, runDeps(doctorDeps(nil), &js, "doctor", "--json"), errChecksFailed)
	var report struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(js.Bytes(), &report), "stdout is the JSON report and nothing else")
	assert.False(t, report.OK)
	assert.NotEmpty(t, report.Checks)
}

func TestDoctorNeverPrintsASecret(t *testing.T) {
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

	for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}} {
		var out bytes.Buffer
		require.ErrorIs(t, runDeps(doctorDeps(echo), &out, args...), errChecksFailed)

		assert.Contains(t, out.String(), "[redacted]", "the error text reached the report, so the redactor was exercised")
		for name, value := range secretValues {
			assert.NotContains(t, out.String(), value, "%s in %v", name, args)
		}
		assert.NotContains(t, out.String(), "DbPasswordSentinel9")
		assert.NotContains(t, out.String(), "SmtpPasswordSentinel7")
	}
}

func TestDoctorTakesNoArguments(t *testing.T) {
	require.ErrorContains(t, runDeps(doctorDeps(nil), &bytes.Buffer{}, "doctor", "extra"), "unexpected argument")
}
