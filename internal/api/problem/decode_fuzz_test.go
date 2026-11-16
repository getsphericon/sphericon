package problem_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-faster/jx"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/problem"
)

// A request body is the client's bytes, and the path of a property it fails at is read from the
// decoder's error, which carries the client's key names, with the item of each array on the way
// found by decoding the body again (ADR 0148). Whatever the body, a body that does not decode is
// a 400, every field error names a path of whole segments that leads somewhere in the body and
// carries a reason, and nothing panics.
func FuzzUndecodedBodyNamesWholePaths(f *testing.F) {
	f.Add(`{"name":"x","segment_id":"a"}`)
	f.Add(`{"name":"x","content":{"channel":"email","subjct":"s"}}`)
	f.Add(`{"name":"x","content":{"channel":"email","subject":3}}`)
	f.Add(`{"name":"x","content":{"channel":"email","a.b\"c":1}}`)
	f.Add(`{"name":"x","content":{"channel":"sms","text":"hi","":1}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"f","operator":"eq","value":1},{"kind":"rule","combinator":"or"}]}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","rules":[{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":3}]}]}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","rules":7}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","rules":[],"rules":[{"1":2}]}}`)
	f.Add(`{"name":`)
	f.Add(`3`)

	f.Fuzz(func(t *testing.T, body string) {
		for _, op := range []operation{broadcastCreate, segmentCreate} {
			status, fields := answerTo(op, body)

			assert.Containsf(t, []int{0, http.StatusBadRequest}, status, "%q", body)
			for path, reasons := range fields {
				assert.Truef(t, leadsInto(body, path), "%q: %q leads nowhere in the body", body, path)
				assert.NotEmptyf(t, reasons, "%q", body)
			}
		}
	})
}

// operation is an /api operation and its request body type.
type operation struct {
	name externalapi.OperationName
	in   func() decodable
}

// decodable is a request body type ogen decodes as JSON.
type decodable interface{ Decode(d *jx.Decoder) error }

var (
	broadcastCreate = operation{externalapi.BroadcastsCreateOperation, func() decodable { return &externalapi.CreateBroadcastInput{} }}
	segmentCreate   = operation{externalapi.SegmentsCreateOperation, func() decodable { return &externalapi.CreateSegmentInput{} }}
)

// answerTo is the status and the field errors the operation is answered with on /api when body
// does not decode, handed to the error handler as ogen's server hands it, or 0 and none when body
// decodes.
func answerTo(op operation, body string) (status int, fields map[string][]string) {
	err := op.in().Decode(jx.DecodeStr(body))
	if err == nil {
		return 0, nil
	}
	refused := &ogenerrors.DecodeRequestError{
		Name: op.name,
		Err:  &ogenerrors.DecodeBodyError{ContentType: "application/json", Body: []byte(body), Err: err},
	}
	h := problem.Middleware[externalapi.Handler](http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var answer *siteapi.ProblemDetails
		status, answer = problem.Of(r.Context(), refused)
		fields = answer.Errors.Or(nil)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody))
	return status, fields
}

// leadsInto reports whether path names a place in body: each key a member of the object it is
// in, each index an item of the array. An array a path crosses without an index (a failure the
// answer could not place in it) leads on through any of its items. A body that is no JSON
// document, which a decoder may stop in before its end, is not checked, nor one that is not UTF-8,
// whose keys encoding/json would read otherwise.
func leadsInto(body, path string) bool {
	var doc any
	if !utf8.ValidString(body) || json.Unmarshal([]byte(body), &doc) != nil {
		return true
	}
	segments, ok := splitPath(path)
	return ok && reaches(doc, segments)
}

func reaches(value any, segments []string) bool {
	if len(segments) == 0 {
		return true
	}
	head, quoted := segments[0], strings.HasPrefix(segments[0], `"`)
	key := head
	if quoted {
		unquoted, err := strconv.Unquote(head)
		if err != nil {
			return false
		}
		key = unquoted
	}
	switch v := value.(type) {
	case map[string]any:
		item, ok := v[key]
		return ok && reaches(item, segments[1:])
	case []any:
		if i, err := strconv.Atoi(head); err == nil && !quoted {
			return i >= 0 && i < len(v) && reaches(v[i], segments[1:])
		}
		for _, item := range v {
			if reaches(item, segments) {
				return true
			}
		}
	}
	return false
}

// splitPath is path's segments, a quoted one kept quoted; ok is false for an empty segment.
func splitPath(path string) (segments []string, ok bool) {
	for rest := path; ; {
		var segment string
		if strings.HasPrefix(rest, `"`) {
			quoted, err := strconv.QuotedPrefix(rest)
			if err != nil {
				return nil, false
			}
			segment, rest = quoted, rest[len(quoted):]
		} else if dot := strings.IndexByte(rest, '.'); dot >= 0 {
			segment, rest = rest[:dot], rest[dot:]
		} else {
			segment, rest = rest, ""
		}
		if segment == "" {
			return nil, false
		}
		segments = append(segments, segment)
		if rest == "" {
			return segments, true
		}
		if rest[0] != '.' || len(rest) == 1 {
			return nil, false
		}
		rest = rest[1:]
	}
}

// A body nested deeper than the item search may decode it again for is still refused, and its
// field error names the keys alone: the work a refused request costs is bounded.
func TestAFailureNestedPastTheSearchNamesItsKeys(t *testing.T) {
	const depth = 80
	condition := `{"kind":"rule","bogus":true}`
	path := "bogus"
	for range depth {
		condition = `{"kind":"group","combinator":"and","rules":[` + condition + `]}`
		path = "rules." + path
	}
	body := `{"name":"x","condition":` + condition + `}`

	status, fields := answerTo(segmentCreate, body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, map[string][]string{"condition." + path: {problem.UnknownProperty}}, fields)
}

// A failure nested within the search names the item of every array on the way.
func TestAFailureNestedWithinTheSearchNamesEveryItem(t *testing.T) {
	const depth = 10
	condition := `{"kind":"rule","bogus":true}`
	path := "bogus"
	for range depth {
		condition = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"f","operator":"eq","value":"v"},` + condition + `]}`
		path = "rules.1." + path
	}
	body := `{"name":"x","condition":` + condition + `}`

	status, fields := answerTo(segmentCreate, body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, map[string][]string{"condition." + path: {problem.UnknownProperty}}, fields)
}
