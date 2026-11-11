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
