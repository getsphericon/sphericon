package problem

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/ogen-go/ogen/validate"
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
// segments joined by dots (`content.subjct`, `condition.entries.1.name`), and the reason it is
// refused: UnknownProperty for a property the contract does not declare, the decoder's own message
// for a value it cannot read (a wrong type). The path is empty when err is no request decoding
// error or names no property. The decoders name the object keys only; the item of each array on
// the way is found by decoding the body again (locate), with the decoder of the operation the
// surface's Middleware keeps, and a failure it cannot place keeps the keys alone.
func undecodedProperty(ctx context.Context, err error) (path, reason string) {
	request, ok := errors.AsType[*ogenerrors.DecodeRequestError](err)
	if !ok {
		return "", ""
	}
	keys, reason, found := propertyOf(err, nil)
	if !found {
		return "", ""
	}
	steps := make([]step, 0, len(keys))
	for _, key := range keys {
		steps = append(steps, step{key: key})
	}
	body, isBody := errors.AsType[*ogenerrors.DecodeBodyError](err)
	if decode := decoderOf(ctx, request.OperationName()); isBody && decode != nil {
		if located, ok := locate(body.Body, failure{keys, reason}, decode); ok {
			steps = located
		}
	}
	return joinPath(steps), reason
}

// propertyOf follows err's chain, a joined error into each of its errors, collecting the keys of
// the fields each level decodes, and stops at the first error that ends the chain under a field.
func propertyOf(err error, segments []string) (keys []string, reason string, found bool) {
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

// segment is a key as a segment of the path: as it is, or Go-quoted when it is empty, holds a dot
// or a quote, or is a number, so that a refused key a client chose (`""`, `a.b`, `1`) cannot read
// as another path or as the index of an item.
func segment(key string) string {
	if key == "" || strings.ContainsAny(key, `."`) || !strconv.CanBackquote(key) || isIndex(key) {
		return strconv.Quote(key)
	}
	return key
}

// isIndex reports whether key reads as the index of an item: digits only.
func isIndex(key string) bool {
	return strings.Trim(key, "0123456789") == ""
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

// unionWrapper is the name ogen's validators give the field that holds a union's variant, a
// field of the generated type that is no key of the body.
const unionWrapper = "OneOf"

// itemName is how ogen's validators name the item of an array they refuse: `[<index>]`.
var itemName = regexp.MustCompile(`^\[(\d+)\]$`)

// invalidProperties is the path and the reason of each property of a decoded request body the
// contract's validation refuses (a value outside an enum, a range or a pattern), its segments joined by dots
// as undecodedProperty joins them (`condition.entries.1.test.operator`). Unlike a decoding error,
// ogen's validators name every key and every array item on the way, so the body is not decoded
// again. It is empty when err is no validation of a request body.
func invalidProperties(err error) map[string][]string {
	if _, ok := errors.AsType[*ogenerrors.DecodeRequestError](err); !ok {
		return nil
	}
	// A body that does not decode (a missing required property among them) is undecodedProperty's,
	// a parameter is no property of the body.
	if _, ok := errors.AsType[*ogenerrors.DecodeBodyError](err); ok {
		return nil
	}
	if _, ok := errors.AsType[*ogenerrors.DecodeParamsError](err); ok {
		return nil
	}
	invalid, ok := errors.AsType[*validate.Error](err)
	if !ok {
		return nil
	}
	out := map[string][]string{}
	collectInvalid(invalid, nil, out)
	return out
}

// collectInvalid adds to out the path and reason of every field invalid refuses, under the path
// steps leads to it.
func collectInvalid(invalid *validate.Error, steps []step, out map[string][]string) {
	for _, field := range invalid.Fields {
		at := steps
		if match := itemName.FindStringSubmatch(field.Name); match != nil {
			index, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}
			at = append(slices.Clip(steps), step{index: index, item: true})
		} else if field.Name != unionWrapper {
			at = append(slices.Clip(steps), step{key: field.Name})
		}
		if nested, ok := errors.AsType[*validate.Error](field.Error); ok {
			collectInvalid(nested, at, out)
			continue
		}
		if len(at) > 0 {
			path := joinPath(at)
			out[path] = append(out[path], field.Error.Error())
		}
	}
}
