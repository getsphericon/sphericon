package problem_test

import (
	"context"
	"encoding/json"
	"fmt"
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
// found by decoding the body again (ADR 0148). Whatever the body, a body that does not decode or
// holds a value the contract does not take is a 400, every field error names a path of whole
// segments that leads somewhere in the body and carries a reason, and nothing panics.
func FuzzUndecodedBodyNamesWholePaths(f *testing.F) {
	f.Add(`{"name":"x","segment_id":"a"}`)
	f.Add(`{"name":"x","content":{"channel":"email","subjct":"s"}}`)
	f.Add(`{"name":"x","content":{"channel":"email","subject":3}}`)
	f.Add(`{"name":"x","content":{"channel":"email","a.b\"c":1}}`)
	f.Add(`{"name":"x","content":{"channel":"sms","text":"hi","":1}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":1,"operator":"has"},{"kind":"tag","combinator":"or"}]}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","entries":[{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":3}]}]}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","entries":7}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"v","operator":"has"},{"kind":"customField","key":"k","test":{"kind":"number","operator":"contains","value":1}}]}}`)
	f.Add(`{"name":"x","condition":{"kind":"group","entries":[],"entries":[{"1":2}]}}`)
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

// decodable is a request body type ogen decodes as JSON and then validates against the contract.
type decodable interface {
	Decode(d *jx.Decoder) error
	Validate() error
}

var (
	broadcastCreate = operation{externalapi.BroadcastsCreateOperation, func() decodable { return &externalapi.CreateBroadcastInput{} }}
	segmentCreate   = operation{externalapi.SegmentsCreateOperation, func() decodable { return &externalapi.CreateSegmentInput{} }}
)

// answerTo is the status and the field errors the operation is answered with on /api when body
// does not decode or holds a value the contract does not take, handed to the error handler as
// ogen's server hands it, or 0 and none when body decodes and validates.
func answerTo(op operation, body string) (status int, fields map[string][]string) {
	refused := refusalOf(op, body)
	if refused == nil {
		return 0, nil
	}
	h := problem.Middleware[externalapi.Handler](http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var answer *siteapi.ProblemDetails
		status, answer = problem.Of(r.Context(), refused)
		fields = answer.Errors.Or(nil)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody))
	return status, fields
}

// refusalOf is the error ogen's server answers body with: its decoding error, else its validation
// error, else nil.
func refusalOf(op operation, body string) error {
	in := op.in()
	if err := in.Decode(jx.DecodeStr(body)); err != nil {
		return &ogenerrors.DecodeRequestError{
			Name: op.name,
			Err:  &ogenerrors.DecodeBodyError{ContentType: "application/json", Body: []byte(body), Err: err},
		}
	}
	if err := in.Validate(); err != nil {
		return &ogenerrors.DecodeRequestError{Name: op.name, Err: fmt.Errorf("validate: %w", err)}
	}
	return nil
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
	condition := `{"kind":"tag","bogus":true}`
	path := "bogus"
	for range depth {
		condition = `{"kind":"group","combinator":"and","entries":[` + condition + `]}`
		path = "entries." + path
	}
	body := `{"name":"x","condition":` + condition + `}`

	status, fields := answerTo(segmentCreate, body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, map[string][]string{"condition." + path: {problem.UnknownProperty}}, fields)
}

// A failure nested within the search names the item of every array on the way.
func TestAFailureNestedWithinTheSearchNamesEveryItem(t *testing.T) {
	const depth = 10
	condition := `{"kind":"tag","bogus":true}`
	path := "bogus"
	for range depth {
		condition = `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"v","operator":"has"},` + condition + `]}`
		path = "entries.1." + path
	}
	body := `{"name":"x","condition":` + condition + `}`

	status, fields := answerTo(segmentCreate, body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, map[string][]string{"condition." + path: {problem.UnknownProperty}}, fields)
}

// A body that decodes but holds a value outside the contract (an operator its test does not take,
// an attribute the Contact has not) names it by its path, the item of every array on the way
// included, as a body that does not decode does. The path passes through two arrays' items and two
// unions (a place's entry and a leaf's test), so it pins the names ogen's generated validators give
// an item (`[<index>]`) and a union's variant (`OneOf`), which the path is read from: an ogen that
// renames either fails here.
func TestAValueOutsideTheContractNamesItsPath(t *testing.T) {
	const body = `{"name":"x","condition":{"kind":"group","combinator":"and","entries":[
		{"kind":"tag","name":"v","operator":"has"},
		{"kind":"group","combinator":"or","entries":[
			{"kind":"customField","key":"k","test":{"kind":"number","operator":"contains","value":1}},
			{"kind":"attribute","attribute":"password","test":{"kind":"text","operator":"eq","value":"x"}}
		]}
	]}}`

	status, fields := answerTo(segmentCreate, body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, map[string][]string{
		"condition.entries.1.entries.0.test.operator": {"invalid value: contains"},
		"condition.entries.1.entries.1.attribute":     {"invalid value: password"},
	}, fields)
}
