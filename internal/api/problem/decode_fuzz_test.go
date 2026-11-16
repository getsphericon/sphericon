package problem_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-faster/jx"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/problem"
)

// A request body is the client's bytes, and the path of a property it fails at is read from the
// decoder's error, which carries the client's key names (ADR 0148). Whatever the body, a body that
// does not decode is a 400, every field error names a path of whole keys and carries a reason, and
// nothing panics.
func FuzzUndecodedBodyNamesWholePaths(f *testing.F) {
	f.Add(`{"name":"x","segment_id":"a"}`)
	f.Add(`{"name":"x","content":{"channel":"email","subjct":"s"}}`)
	f.Add(`{"name":"x","content":{"channel":"email","subject":3}}`)
	f.Add(`{"name":"x","content":{"channel":"email","a.b\"c":1}}`)
	f.Add(`{"name":"x","content":{"channel":"sms","text":"hi","":1}}`)
	f.Add(`{"name":`)
	f.Add(`3`)

	f.Fuzz(func(t *testing.T, body string) {
		status, fields := answerToBody(body)

		assert.Containsf(t, []int{0, http.StatusBadRequest}, status, "%q", body)
		for path, reasons := range fields {
			assert.NotEmptyf(t, path, "%q", body)
			assert.NotContainsf(t, strings.Split(path, "."), "", "%q: %q has an empty segment", body, path)
			assert.NotEmptyf(t, reasons, "%q", body)
		}
	})
}

// answerToBody is the status and the field errors a Broadcast create of body is answered with when
// it does not decode, or 0 and none when it does.
func answerToBody(body string) (status int, fields map[string][]string) {
	var in externalapi.CreateBroadcastInput
	err := in.Decode(jx.DecodeStr(body))
	if err == nil {
		return 0, nil
	}
	status, answer := problem.Of(context.Background(), &ogenerrors.DecodeRequestError{Err: err})
	return status, answer.Errors.Or(nil)
}
