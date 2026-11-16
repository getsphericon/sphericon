package testhelper

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Resend is what a client that read v sends back: v's JSON decoded as the request type In. A
// response and a request have models of their own, the response's open and the request's closed
// (ADR 0148), so a test that edits what it read and writes it back goes through the wire format, as
// the client does.
func Resend[In any, PIn interface {
	*In
	UnmarshalJSON(data []byte) error
}](tb testing.TB, v json.Marshaler) In {
	tb.Helper()
	raw, err := v.MarshalJSON()
	require.NoError(tb, err)
	var in In
	require.NoError(tb, PIn(&in).UnmarshalJSON(raw))
	return in
}

// JSONOf is v as a client reads it on the wire: a request's model and a response's are types of
// their own, so a test that compares what it sent with what it read compares their JSON.
func JSONOf(tb testing.TB, v json.Marshaler) string {
	tb.Helper()
	raw, err := v.MarshalJSON()
	require.NoError(tb, err)
	return string(raw)
}
