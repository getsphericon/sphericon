package emailrender_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// A From whose local part is a no-reply word refuses the replies that tell mailbox providers a
// message is wanted (ADR 0188); a word that only starts like one, or no From at all, is not.
func TestLintWarnsOnANoReplySender(t *testing.T) {
	for from, want := range map[string][]string{
		"no-reply@acme.com":     {"no-reply@acme.com"},
		"noreply@acme.com":      {"noreply@acme.com"},
		"DoNotReply@acme.com":   {"DoNotReply@acme.com"},
		"do-not-reply@acme.com": {"do-not-reply@acme.com"},
		"reply@acme.com":        {},
		"noreplies@acme.com":    {},
		"hello@no-reply.com":    {},
		"":                      {},
	} {
		t.Run(from, func(t *testing.T) {
			report := emailrender.Lint(emailrender.Message{Subject: "Hi", Body: "<p>Hello</p>", From: from})
			require.NoError(t, report.Error)
			assert.Equal(t, want, details(report, emailrender.CodeNoReplySender))
		})
	}
}

// A document's preview text equal to its subject, once trimmed and case-folded, wastes the line a
// recipient reads next (ADR 0188); a message with no preview text (a raw HTML body) has nothing to
// compare.
func TestLintWarnsOnAPreviewTextThatRepeatsTheSubject(t *testing.T) {
	for name, tc := range map[string]struct {
		subject, preview string
		want             []string
	}{
		"the same words":       {subject: "sale ends today ", preview: "Sale ends today", want: []string{"Sale ends today"}},
		"a different preview":  {subject: "Sale ends today", preview: "Last chance for 20% off", want: []string{}},
		"no preview text":      {subject: "Sale ends today", want: []string{}},
		"neither of them":      {want: []string{}},
		"a blank subject only": {subject: "  ", preview: "Sale ends today", want: []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			report := emailrender.Lint(emailrender.Message{Subject: tc.subject, Body: "<p>Hello</p>", PreviewText: tc.preview})
			require.NoError(t, report.Error)
			assert.Equal(t, tc.want, details(report, emailrender.CodePreviewRepeatsSubject))
		})
	}
}

// The message's own findings come first, so a body that fills the report's cap cannot crowd them out.
func TestLintKeepsTheSenderFindingInAFullReport(t *testing.T) {
	var body strings.Builder
	for i := range 80 {
		fmt.Fprintf(&body, `<img src="https://cdn.acme.com/%d.png">`, i)
	}
	report := emailrender.Lint(emailrender.Message{Subject: "Hi", Body: body.String(), From: "noreply@acme.com"})
	require.NoError(t, report.Error)
	assert.Len(t, report.Warnings, 50)
	assert.Equal(t, []string{"noreply@acme.com"}, details(report, emailrender.CodeNoReplySender))
}
