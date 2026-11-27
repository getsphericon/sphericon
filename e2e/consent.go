//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
)

// OneClickEncoding is how a mailbox provider encodes the one-click POST body. RFC 8058 §3.1
// allows both, and providers differ, so the endpoint must take either.
type OneClickEncoding int

const (
	// OneClickMultipart is multipart/form-data, the encoding RFC 8058 recommends.
	OneClickMultipart OneClickEncoding = iota
	// OneClickURLEncoded is application/x-www-form-urlencoded.
	OneClickURLEncoded
)

// OneClickUnsubscribe performs the RFC 8058 one-click request exactly as a mailbox
// provider would: it uses only the https URL from the message's List-Unsubscribe
// header and the body from List-Unsubscribe-Post, sends it in the given encoding, and
// requires success (2xx).
func (w *Workspace) OneClickUnsubscribe(msg Message, enc OneClickEncoding) {
	w.t.Helper()
	target := msg.UnsubscribeURL()
	require.NotEmpty(w.t, target, "List-Unsubscribe carries an http(s) URL, got %q", msg.Header("List-Unsubscribe"))
	key, value, found := strings.Cut(msg.OneClickBody(), "=")
	require.True(w.t, found && key == "List-Unsubscribe" && value == "One-Click",
		"List-Unsubscribe-Post announces one-click, got %q", msg.OneClickBody())

	var body bytes.Buffer
	contentType := "application/x-www-form-urlencoded"
	switch enc {
	case OneClickMultipart:
		form := multipart.NewWriter(&body)
		require.NoError(w.t, form.WriteField(key, value))
		require.NoError(w.t, form.Close())
		contentType = form.FormDataContentType()
	case OneClickURLEncoded:
		body.WriteString(msg.OneClickBody())
	}
	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodPost, target, &body)
	require.NoError(w.t, err)
	req.Header.Set("Content-Type", contentType)
	res, err := http.DefaultClient.Do(req)
	require.NoError(w.t, err)
	out, err := io.ReadAll(res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Equalf(w.t, 2, res.StatusCode/100, "one-click POST to %s answered %d: %s", target, res.StatusCode, out)
}

// PrefetchUnsubscribe requests the message's List-Unsubscribe URL the way a link scanner or a
// mail client's preview does, with HEAD and then GET and without following redirects, and requires
// that neither fails. A safe request must never opt the Contact out.
func (w *Workspace) PrefetchUnsubscribe(msg Message) {
	w.t.Helper()
	target := msg.UnsubscribeURL()
	require.NotEmpty(w.t, target, "List-Unsubscribe carries an http(s) URL")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequestWithContext(w.t.Context(), method, target, http.NoBody)
		require.NoError(w.t, err)
		res, err := client.Do(req)
		require.NoError(w.t, err)
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(w.t, err)
		require.NoError(w.t, res.Body.Close())
		require.Less(w.t, res.StatusCode, http.StatusBadRequest, "%s %s answered %d", method, target, res.StatusCode)
	}
}

// UnsubscribeFromFooter is the recipient pressing the unsubscribe link in the email's footer: the
// link answers 303 to the confirmation page, which reads its token from its own address, and the
// page's Confirm button performs the opt-out through the site API, as a visitor who is not signed in.
func (w *Workspace) UnsubscribeFromFooter(msg Message) {
	w.t.Helper()
	link := msg.Link("/e/u/")
	require.NotEmpty(w.t, link, "the email %q carries an unsubscribe link in its footer", msg.Subject)
	page := w.openLink(link, http.StatusSeeOther)
	token := w.pageToken(page, "/unsubscribe")
	err := w.anonymousSite().SitePublicUnsubscribesPerform(w.t.Context(), siteapi.SitePublicUnsubscribesPerformParams{Token: token})
	require.NoError(w.t, err, "the confirmation page's Confirm")
}

// RequireUnsubscribed asserts, through the external API, that the Contact with this
// email has opted out of Broadcasts. The API reads it from the Contact's data export
// (its opt-outs); the opt-out is recorded asynchronously, so it is polled.
func (w *Workspace) RequireUnsubscribed(email string) {
	w.t.Helper()
	require.Eventually(w.t, func() bool {
		doc, found := w.contactExport(email)
		return found && lo.ContainsBy(doc.Unsubscribes, func(u externalapi.ContactExportUnsubscribe) bool {
			return strings.EqualFold(u.Destination, email) && u.SendingSource == eligibility.SourceBroadcasts
		})
	}, EmailTimeout, 100*time.Millisecond, "%s never read as unsubscribed from broadcasts", email)
}

// contactExport reads the data export of the Contact with this email through the external API.
// It reports false while the Contact cannot be read yet.
func (w *Workspace) contactExport(email string) (externalapi.ContactExportDocument, bool) {
	w.t.Helper()
	var doc externalapi.ContactExportDocument
	res, err := w.api.ContactsExport(w.t.Context(), externalapi.ContactsExportParams{Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(email))})
	if err != nil {
		return doc, false
	}
	dl, isDL := res.(*externalapi.ContactsExportOKApplicationOctetStreamHeaders)
	if !isDL {
		return doc, false
	}
	raw, err := io.ReadAll(dl.Response)
	if err != nil {
		return doc, false
	}
	return doc, doc.Decode(jx.DecodeBytes(raw)) == nil
}

// RequireConfirmedOptIn sets up the Workspace's confirmation email, from its own From and with
// subject, and turns confirmed opt-in on, as its owner does in the settings: only a person turns the
// policy on (ADR 0013), so the step goes through the SPA's API.
func (w *Workspace) RequireConfirmedOptIn(subject string) {
	w.t.Helper()
	ctx := w.t.Context()
	id := siteapi.WorkspaceId(w.publicID)
	revision := func() siteapi.Revision {
		read, err := w.site.SiteWorkspacesGet(ctx, siteapi.SiteWorkspacesGetParams{WorkspaceId: id})
		require.NoError(w.t, err, "read the workspace")
		return read.Revision
	}
	_, err := w.site.SiteWorkspacesUpdateConfirmationEmail(ctx, &siteapi.SiteUpdateConfirmationEmailInput{
		Revision: revision(), FromEmail: siteapi.EmailAddress(w.FromEmail), Subject: siteapi.NewOptString(subject),
	}, siteapi.SiteWorkspacesUpdateConfirmationEmailParams{WorkspaceId: id})
	require.NoError(w.t, err, "set up the confirmation email")
	_, err = w.site.SiteWorkspacesSetConfirmedOptIn(ctx, &siteapi.SiteSetConfirmedOptInInput{Revision: revision(), Enabled: true},
		siteapi.SiteWorkspacesSetConfirmedOptInParams{WorkspaceId: id})
	require.NoError(w.t, err, "turn confirmed opt-in on")
}

// RequestConfirmation asks, through the external API, for a confirmation email to the Contact with
// this email, which ImportContacts made.
func (w *Workspace) RequestConfirmation(email string) {
	w.t.Helper()
	res, err := w.api.ContactsBatchUpsert(w.t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{{
		Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(email)),
	}}})
	out := ok[externalapi.UpsertContactsResult](w.t, "find the contact", res, err)
	require.Len(w.t, out.Results, 1)
	contactID, found := out.Results[0].ContactId.Get()
	require.True(w.t, found, "the contact of %s", email)
	asked, err := w.api.ConfirmationRequestsCreate(w.t.Context(), externalapi.ConfirmationRequestsCreateParams{ContactId: contactID})
	ok[externalapi.ConfirmationRequestsCreateAccepted](w.t, "request a confirmation email", asked, err)
}

// ConfirmFromEmail is the recipient pressing the Confirm button of a confirmation email: the link
// answers 303 to the confirmation page, which reads its token from its own address, and the page's
// Confirm button performs the confirmation through the site API, as a visitor who is not signed in.
func (w *Workspace) ConfirmFromEmail(msg Message) {
	w.t.Helper()
	link := msg.Link("/e/confirm/")
	require.NotEmpty(w.t, link, "the email %q carries a confirmation link", msg.Subject)
	page := w.openLink(link, http.StatusSeeOther)
	token := w.pageToken(page, "/confirm")
	err := w.anonymousSite().SitePublicConfirmationsPerform(w.t.Context(), siteapi.SitePublicConfirmationsPerformParams{Token: token})
	require.NoError(w.t, err, "the confirmation page's Confirm")
}

// ImportRefusals imports a CSV file of Contacts as ImportCSV does, but with its column at mapped
// as a refusal column of email: a cell that is one of values records an Unsubscribe of
// everything, `via: imported` (ADR 0138). It returns the import as it ended.
func (w *Workspace) ImportRefusals(file string, at int, values ...string) *siteapi.SiteContactImport {
	w.t.Helper()
	ctx := w.t.Context()
	ws := siteapi.WorkspaceId(w.publicID)
	uploaded, err := w.site.SiteContactImportsUpload(ctx, siteapi.SiteContactImportsUploadReq{Data: strings.NewReader(file)},
		siteapi.SiteContactImportsUploadParams{WorkspaceId: ws})
	require.NoError(w.t, err, "upload the file")
	raw, err := uploaded.Mapping.MarshalJSON()
	require.NoError(w.t, err)
	var mapping siteapi.ContactImportMappingInput
	require.NoError(w.t, mapping.UnmarshalJSON(raw), "the proposal as a request saves it")
	mapping.Columns[at] = siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportRefusalColumnInputContactImportColumnInputSum(siteapi.ContactImportRefusalColumnInput{
		Kind: siteapi.ContactImportRefusalColumnInputKindRefusal, Channel: siteapi.ContactImportRefusalChannelEmail,
		Values: lo.Map(values, func(v string, _ int) siteapi.ContactImportRefusalValueInput {
			return siteapi.ContactImportRefusalValueInput{Value: v, Refusal: siteapi.ContactImportRefusalUnsubscribeEverything}
		}),
	})}
	_, err = w.site.SiteContactImportsSaveMapping(ctx, &mapping, siteapi.SiteContactImportsSaveMappingParams{WorkspaceId: ws, ImportId: uploaded.ID})
	require.NoError(w.t, err, "save the mapping")
	preview, err := w.site.SiteContactImportsPreview(ctx, siteapi.SiteContactImportsPreviewParams{WorkspaceId: ws, ImportId: uploaded.ID})
	require.NoError(w.t, err, "preview the import")
	require.Positive(w.t, preview.OptOuts, "the preview counts the opt-outs the file records")
	_, err = w.site.SiteContactImportsRun(ctx, siteapi.SiteContactImportsRunParams{WorkspaceId: ws, ImportId: uploaded.ID})
	require.NoError(w.t, err, "run the import")

	var done *siteapi.SiteContactImport
	require.Eventually(w.t, func() bool {
		imp, err := w.site.SiteContactImportsGet(ctx, siteapi.SiteContactImportsGetParams{WorkspaceId: ws, ImportId: uploaded.ID})
		done = imp
		return err == nil && imp.State == siteapi.ContactImportStateDone
	}, EmailTimeout, 100*time.Millisecond, "the import of %s never ended done", w)
	return done
}
