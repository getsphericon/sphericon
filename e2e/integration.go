//go:build e2e

package e2e

import (
	"time"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// CreateMailpitIntegration creates the default SMTP Integration pointing at the
// suite's Mailpit, sending as FromName <FromEmail>.
func (w *Workspace) CreateMailpitIntegration() {
	w.t.Helper()
	cfg := externalapi.SmtpConfigInput{
		Kind:     externalapi.SmtpConfigInputKindSMTP,
		Host:     w.env.mailpit.SMTPHost,
		Port:     narrow.Int32(w.env.mailpit.SMTPPort),
		From:     externalapi.EmailAddress(w.FromEmail),
		FromName: externalapi.NewOptNilString(w.FromName),
	}
	res, err := w.api.IntegrationsCreate(w.t.Context(), &externalapi.CreateIntegrationInput{
		Name:      "mailpit",
		IsDefault: externalapi.NewOptBool(true),
		Config:    externalapi.IntegrationConfigInput{OneOf: externalapi.NewSmtpConfigInputIntegrationConfigInputSum(cfg)},
	})
	ok[externalapi.IntegrationResource](w.t, "create integration", res, err)
}

// AddVerifiedSendingDomain creates the unique Sending domain, triggers verification
// (an asynchronous job) and polls until it reads as verified.
func (w *Workspace) AddVerifiedSendingDomain() {
	w.t.Helper()
	ctx := w.t.Context()
	res, err := w.api.SendingDomainsCreate(ctx, &externalapi.CreateSendingDomainInput{Domain: w.Domain})
	sd := ok[externalapi.SendingDomainResource](w.t, "create sending domain", res, err)

	_, err = w.api.SendingDomainsVerify(ctx, externalapi.SendingDomainsVerifyParams{ID: sd.ID})
	require.NoError(w.t, err, "verify sending domain") // 202: the check is a job; the poll below is the assertion

	require.Eventually(w.t, func() bool {
		got, err := w.api.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: sd.ID})
		if err != nil {
			return false
		}
		d, isD := got.(*externalapi.SendingDomainResource)
		return isD && d.Verified
	}, EmailTimeout, 100*time.Millisecond, "Sending domain %s never became verified", w.Domain)
}
