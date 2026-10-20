package doctor_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/doctor"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var now = time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

// healthy builds the Input of an Instance on which every check passes, against the test
// database at the fixture state; a test then breaks one thing.
func healthy(t *testing.T) doctor.Input {
	t.Helper()
	app := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(app.Close)
	key, err := secrets.GenerateKeysetBase64()
	require.NoError(t, err)

	sqlDB := testhelper.RealDB(t)
	entClient := appdb.NewEntClient(sqlDB)
	domains, err := entClient.SendingDomain.Query().All(context.Background())
	require.NoError(t, err)
	keys := map[string][]string{} // two Workspaces may claim one name, each with its own key
	for _, d := range domains {
		host, _ := sending.DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
		keys[host+"."] = append(keys[host+"."], d.DkimPublicKey)
	}

	return doctor.Input{
		Config: &config.Config{
			AppURL:                   app.URL,
			EncryptionKey:            key,
			JWTSecret:                strings.Repeat("k3", 20),
			SMTPHost:                 "smtp.example.test",
			SMTPPort:                 587,
			SMTPUser:                 "mailer",
			SystemEmailProvider:      "smtp",
			RequireEmailVerification: true,
		},
		DB:       sqlDB,
		Ent:      entClient,
		Build:    buildinfo.Info{Version: "v1.0.0"},
		Now:      func() time.Time { return now },
		DialSMTP: func(context.Context, *config.Config) error { return nil },
		HTTP:     app.Client(),
		LookupTXT: func(_ context.Context, name string) ([]string, error) {
			if k, ok := keys[name]; ok {
				return k, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
		},
		LicensePublicKey: licensekey.ProductionKey,
	}
}

func statuses(r doctor.Report) map[string]doctor.Status {
	out := map[string]doctor.Status{}
	for _, c := range r.Checks {
		out[c.Name] = c.Status
	}
	return out
}

func find(t *testing.T, r doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	require.Failf(t, "check not in the report", "%q in %v", name, statuses(r))
	return doctor.Check{}
}

func TestHealthyInstancePassesEveryCheckThatCanFail(t *testing.T) {
	in := healthy(t)
	r := doctor.Run(context.Background(), in)

	for name, status := range statuses(r) {
		if strings.HasPrefix(name, "dkim ") {
			continue // the fixture domains are not all published; see the DKIM test
		}
		assert.NotEqual(t, doctor.Fail, status, name)
	}
	assert.Equal(t, doctor.Pass, find(t, r, "database").Status)
	assert.Equal(t, doctor.Pass, find(t, r, "postgres version").Status)
	assert.Equal(t, doctor.Pass, find(t, r, "app url reachable").Status)
}

func TestConfigurationThatDoesNotLoadFailsWithItsReason(t *testing.T) {
	r := doctor.Run(context.Background(), doctor.Input{ConfigErr: errors.New("SPHERICON_JWT_SECRET is required")})

	require.True(t, r.Failed())
	c := find(t, r, "configuration")
	assert.Contains(t, c.Detail, "SPHERICON_JWT_SECRET")
	assert.NotEmpty(t, c.Fix)
}

func TestUnreachableDatabaseFailsEveryCheckThatNeedsIt(t *testing.T) {
	in := healthy(t)
	in.DB, in.Ent, in.DBErr = nil, nil, errors.New("connection refused")
	r := doctor.Run(context.Background(), in)

	require.True(t, r.Failed())
	assert.Contains(t, find(t, r, "database").Detail, "connection refused")
	assert.Equal(t, doctor.Fail, find(t, r, "postgres version").Status)
	assert.Equal(t, doctor.Fail, find(t, r, "dkim").Status)
}

func TestSchemaNewerThanTheBinaryFailsWithTheRemedy(t *testing.T) {
	in := healthy(t)
	in.Build = buildinfo.Info{Version: "v0.0.1"} // older than the floor of the database's Major
	r := doctor.Run(context.Background(), in)

	c := find(t, r, "database")
	assert.Equal(t, doctor.Fail, c.Status)
	assert.Contains(t, c.Detail, "older than the database allows")
}

func TestSMTPFailureFailsWithAFix(t *testing.T) {
	in := healthy(t)
	in.DialSMTP = func(context.Context, *config.Config) error { return errors.New("535 authentication failed") }
	c := find(t, doctor.Run(context.Background(), in), "system mail")

	assert.Equal(t, doctor.Fail, c.Status)
	assert.Contains(t, c.Detail, "535 authentication failed")
	assert.Contains(t, c.Fix, "SPHERICON_SMTP_PASS")
}

func TestMissingSMTPHostFailsOnlyWhileVerificationNeedsMail(t *testing.T) {
	in := healthy(t)
	in.Config.SMTPHost = ""
	assert.Equal(t, doctor.Fail, find(t, doctor.Run(context.Background(), in), "system mail").Status)

	in.Config.RequireEmailVerification = false
	assert.Equal(t, doctor.Skip, find(t, doctor.Run(context.Background(), in), "system mail").Status)
}

func TestAppURLChecks(t *testing.T) {
	for name, tc := range map[string]struct {
		url          string
		wellFormed   doctor.Status
		https        doctor.Status
		reachable    doctor.Status
		reachableURL bool
	}{
		"path":           {url: "https://sphericon.example.com/app", wellFormed: doctor.Fail, https: doctor.Skip, reachable: doctor.Skip},
		"no scheme":      {url: "sphericon.example.com", wellFormed: doctor.Fail, https: doctor.Skip, reachable: doctor.Skip},
		"plain http":     {url: "http://sphericon.example.com", wellFormed: doctor.Pass, https: doctor.Fail, reachable: doctor.Fail},
		"local http":     {url: "http://localhost:3000", wellFormed: doctor.Pass, https: doctor.Warn, reachable: doctor.Fail},
		"unreachable":    {url: "https://sphericon.example.com", wellFormed: doctor.Pass, https: doctor.Pass, reachable: doctor.Fail},
		"trailing slash": {url: "https://sphericon.example.com/", wellFormed: doctor.Pass, https: doctor.Pass, reachable: doctor.Fail},
	} {
		t.Run(name, func(t *testing.T) {
			in := healthy(t)
			in.Config.AppURL = tc.url
			in.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("no route to host")
			})}
			r := doctor.Run(context.Background(), in)

			assert.Equal(t, tc.wellFormed, find(t, r, "app url").Status)
			assert.Equal(t, tc.https, find(t, r, "app url https").Status)
			assert.Equal(t, tc.reachable, find(t, r, "app url reachable").Status)
		})
	}
}

func TestAppURLThatAnswersNon200FailsReachability(t *testing.T) {
	in := healthy(t)
	down := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(down.Close)
	in.Config.AppURL, in.HTTP = down.URL, down.Client()

	c := find(t, doctor.Run(context.Background(), in), "app url reachable")
	assert.Equal(t, doctor.Fail, c.Status)
	assert.Contains(t, c.Detail, "502")
}

func TestKeysAreCheckedForPresenceAndValidity(t *testing.T) {
	in := healthy(t)
	in.Config.EncryptionKey = ""
	in.Config.JWTSecret = "short"
	r := doctor.Run(context.Background(), in)
	assert.Equal(t, doctor.Fail, find(t, r, "encryption key").Status)
	assert.Contains(t, find(t, r, "encryption key").Fix, "sphericon genkey")
	assert.Equal(t, doctor.Fail, find(t, r, "jwt secret").Status)

	in.Config.EncryptionKey = "not-a-keyset"
	assert.Contains(t, find(t, doctor.Run(context.Background(), in), "encryption key").Detail, "not a valid keyset")
}

func TestLicenseKeyStates(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	issue := func(expires *time.Time) string {
		key, err := licensekey.Issue(priv, expires, licensekey.FeatureAudit)
		require.NoError(t, err)
		return key
	}
	past, future := now.Add(-24*time.Hour), now.Add(24*time.Hour)

	for name, tc := range map[string]struct {
		key    string
		status doctor.Status
		detail string
	}{
		"none":    {key: "", status: doctor.Pass, detail: "none"},
		"valid":   {key: issue(&future), status: doctor.Pass, detail: "audit"},
		"expired": {key: issue(&past), status: doctor.Warn, detail: "expired on 2026-10-18"},
		"invalid": {key: "not.a.key", status: doctor.Fail, detail: "not valid"},
	} {
		t.Run(name, func(t *testing.T) {
			in := healthy(t)
			in.Config.LicenseKey, in.LicensePublicKey = tc.key, pub
			c := find(t, doctor.Run(context.Background(), in), "license key")

			assert.Equal(t, tc.status, c.Status)
			assert.Contains(t, c.Detail, tc.detail)
		})
	}
}

func TestDKIMIsCheckedForEverySendingDomain(t *testing.T) {
	in := healthy(t)
	r := doctor.Run(context.Background(), in)

	// The stub publishes every fixture domain's record, so all pass.
	var dkim int
	for _, c := range r.Checks {
		if strings.HasPrefix(c.Name, "dkim ") {
			dkim++
			assert.Equal(t, doctor.Pass, c.Status, c.Name)
		}
	}
	assert.NotZero(t, dkim)

	in.LookupTXT = func(_ context.Context, name string) ([]string, error) {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	missing := find(t, doctor.Run(context.Background(), in), "dkim mail.acme.com")
	assert.Equal(t, doctor.Fail, missing.Status)
	assert.Contains(t, missing.Fix, "Publish the TXT record")

	in.LookupTXT = func(context.Context, string) ([]string, error) { return nil, errors.New("server misbehaving") }
	assert.Contains(t, find(t, doctor.Run(context.Background(), in), "dkim mail.acme.com").Detail, "server misbehaving")
}

func TestMailSentFromAnotherOriginWarns(t *testing.T) {
	in := healthy(t)
	moved := find(t, doctor.Run(context.Background(), in), "app url in sent mail")
	assert.Equal(t, doctor.Warn, moved.Status)
	assert.Contains(t, moved.Detail, "https://old.initech.test")
	assert.Contains(t, moved.Fix, "redirect")

	// Pointing the Instance back at that origin clears the warning.
	in.Config.AppURL = "https://OLD.initech.test/"
	assert.Equal(t, doctor.Pass, find(t, doctor.Run(context.Background(), in), "app url in sent mail").Status)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRedactorHidesEverySecretInBothFormats(t *testing.T) {
	var out bytes.Buffer
	w := doctor.Redactor(&out, []string{"hunter2-hunter2", "p@ss\"word-1", "abc"})
	r := doctor.Report{Checks: []doctor.Check{{
		Name: "database", Status: doctor.Fail, Detail: `dial postgres://u:hunter2-hunter2@db/x: refused; pw p@ss"word-1; abc`,
	}}}

	r.WriteText(w)
	require.NoError(t, r.WriteJSON(w))

	assert.NotContains(t, out.String(), "hunter2-hunter2")
	assert.NotContains(t, out.String(), "p@ss")
	assert.Contains(t, out.String(), "[redacted]")
	assert.Contains(t, out.String(), "abc", "a value under four characters is left alone")
}

func TestADeletedWorkspaceIsLeftOutOfTheWorkspaceChecks(t *testing.T) {
	in := healthy(t)
	in.Ent.Workspace.UpdateOneID(fixtures.AcmeID).SetDeletedAt(time.Now()).ExecX(context.Background())
	in.Ent.Workspace.UpdateOneID(fixtures.InitechID).SetDeletedAt(time.Now()).ExecX(context.Background())

	r := doctor.Run(context.Background(), in)

	for _, c := range r.Checks {
		assert.NotEqual(t, "dkim mail.acme.com", c.Name)
	}
	assert.NotEqual(t, doctor.Warn, find(t, r, "app url in sent mail").Status, "mail of a deleted Workspace is not read")
}
