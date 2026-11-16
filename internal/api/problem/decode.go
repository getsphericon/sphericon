package problem

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ogen-go/ogen/ogenerrors"
)

// UnknownProperty is the field error of a property the contract does not declare: a request body
// is a closed object at every depth (ADR 0148).
const UnknownProperty = "unknown property: the contract does not declare it"

// The messages of ogen's generated decoders: each object level wraps the error of a field it
// decodes as `decode field "<name>"`, and a closed object refuses a key it does not know as
// `unexpected field "<name>"`. The names are Go-quoted.
var (
	decodeFieldMessage     = regexp.MustCompile(`^decode field ("(?:[^"\\]|\\.)*")$`)
	unexpectedFieldMessage = regexp.MustCompile(`^unexpected field ("(?:[^"\\]|\\.)*")$`)
)

// undecodedProperty is the path of the request body property a decoding error is about, its
// object keys joined by dots (`content.subjct`), and the reason it is refused: UnknownProperty for
// a property the contract does not declare, the decoder's own message for a value it cannot read
// (a wrong type). The path is empty when err is no request decoding error or names no property. An
// array adds no segment: the decoders do not say which item failed.
func undecodedProperty(err error) (path, reason string) {
	if _, ok := errors.AsType[*ogenerrors.DecodeRequestError](err); !ok {
		return "", ""
	}
	segments, reason, _ := propertyOf(err, nil)
	return strings.Join(segments, "."), reason
}

// propertyOf follows err's chain, a joined error into each of its errors, collecting the fields
// each level decodes, and stops at the first error that ends the chain under a field.
func propertyOf(err error, segments []string) (path []string, reason string, found bool) {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			if path, reason, found := propertyOf(e, segments); found {
				return path, reason, true
			}
		}
		return nil, "", false
	}
	inner := errors.Unwrap(err)
	own := err.Error()
	if inner != nil {
		own = strings.TrimSuffix(own, ": "+inner.Error())
	}
	if name, ok := quoted(unexpectedFieldMessage, own); ok {
		return append(slices.Clip(segments), name), UnknownProperty, true
	}
	if name, ok := quoted(decodeFieldMessage, own); ok {
		segments = append(slices.Clip(segments), name)
	}
	if inner != nil {
		return propertyOf(inner, segments)
	}
	return segments, own, len(segments) > 0
}

// quoted is the Go-quoted name message carries in the only group of pattern.
func quoted(pattern *regexp.Regexp, message string) (string, bool) {
	match := pattern.FindStringSubmatch(message)
	if match == nil {
		return "", false
	}
	name, err := strconv.Unquote(match[1])
	return name, err == nil
}
