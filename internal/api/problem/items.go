package problem

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"

	"github.com/go-faster/jx"
)

// bodyDecoder decodes a request body into the operation's request type, as ogen's server does
// before a handler runs.
type bodyDecoder func(body []byte) error

// requestDecoders are the request body decoders of a surface by operation name, read off the
// parameters of its Handler interface H: an operation's method takes the request body after the
// context, and a body ogen decodes as JSON has a Decode(*jx.Decoder) method.
func requestDecoders[H any]() map[string]bodyDecoder {
	type decodable interface{ Decode(d *jx.Decoder) error }
	decoders := map[string]bodyDecoder{}
	handler := reflect.TypeFor[H]()
	for method := range handler.Methods() {
		if method.Type.NumIn() < 2 {
			continue
		}
		body := method.Type.In(1)
		if body.Kind() == reflect.Pointer {
			body = body.Elem()
		}
		if !reflect.PointerTo(body).Implements(reflect.TypeFor[decodable]()) {
			continue
		}
		decoders[method.Name] = func(raw []byte) error {
			in, ok := reflect.TypeAssert[decodable](reflect.New(body))
			if !ok {
				return errNotInPlace
			}
			return in.Decode(jx.DecodeBytes(raw))
		}
	}
	return decoders
}

// decoderOf is the body decoder of the operation named op on the surface whose Middleware
// answers the request, or nil.
func decoderOf(ctx context.Context, op string) bodyDecoder {
	ex, _ := ctx.Value(exchangeKey{}).(*exchange)
	if ex == nil {
		return nil
	}
	return ex.decoders[op]
}

// step is a segment of the path of a request body property: an object key, or the index of an
// array item when key is unset.
type step struct {
	key   string
	index int
	item  bool
}

// failure is what a decoding error says it failed at: the object keys on the way, without the
// items of the arrays it crossed, and the reason.
type failure struct {
	keys   []string
	reason string
}

func (f failure) equal(o failure) bool { return f.reason == o.reason && slices.Equal(f.keys, o.keys) }

// locate is the path of the failure in body with the index of every array item on the way, found
// by decoding body again with decode, its arrays cut: decoding goes through a body in order and
// stops at its first failure, so the failure is in an array's item i exactly when the body with
// the array cut to its first i+1 items still fails with it, and a binary search finds the
// smallest such cut. Each array found is left cut to the item, so a later array of the same
// path cannot answer for it. ok is false when the failure cannot be placed: decode does not fail
// with it, a key on the way is not once in its object, or the decodes it takes are more than
// locateBudget (a Condition nests without a bound, and a body is decoded again at every array).
func locate(body []byte, f failure, decode bodyDecoder) (path []step, ok bool) {
	budget := locateBudget
	fails := func(body []byte) bool {
		budget--
		return budget >= 0 && failsWith(body, f, decode)
	}
	if !fails(body) {
		return nil, false
	}
	at := span{0, len(body)}
	keys := f.keys
	for {
		value := bytes.TrimLeft(body[at.start:at.end], " \t\r\n")
		if len(value) == 0 {
			return nil, false
		}
		switch value[0] {
		case '[':
			items, ok := itemsOf(body, at)
			if !ok {
				return nil, false
			}
			cut := smallest(len(items)+1, func(n int) bool { return fails(cutTo(body, at, items, n)) })
			switch {
			case budget < 0, cut > len(items):
				return nil, false
			case cut == 0:
				// The array itself is what fails (a value of the wrong type), not an item of it.
				return path, len(keys) == 0
			}
			i := cut - 1
			body, at = cutTo(body, at, items, cut), items[i]
			path = append(path, step{index: i, item: true})
		case '{':
			if len(keys) == 0 {
				return path, true
			}
			value, ok := valueOf(body, at, keys[0])
			if !ok {
				return nil, false
			}
			path = append(path, step{key: keys[0]})
			keys = keys[1:]
			if len(keys) == 0 && f.reason == UnknownProperty {
				// An unknown property's value is never decoded: the path ends at its key.
				return path, true
			}
			at = value
		default:
			return path, len(keys) == 0
		}
	}
}

// locateBudget is how many times locate decodes a body at most: enough for a batch of a thousand
// items in a few nested arrays or a Condition nested ten groups deep, and a bound on the work a
// refused request costs.
const locateBudget = 64

// failsWith reports whether decoding body fails with f.
func failsWith(body []byte, f failure, decode bodyDecoder) bool {
	err := decode(body)
	if err == nil {
		return false
	}
	keys, reason, found := propertyOf(err, nil)
	return found && f.equal(failure{keys, reason})
}

// smallest is the smallest n in [0, count) for which cuts(n) holds, or count when none does; cuts
// holds for every n from the smallest on.
func smallest(count int, cuts func(n int) bool) int {
	lo, hi := 0, count
	for lo < hi {
		mid := lo + (hi-lo)/2
		if cuts(mid) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// span is where a JSON value lies in a body: body[start:end].
type span struct{ start, end int }

// spanIn is where raw, a value jx read from a slice of body, lies in body. jx reads a value of a
// byte slice in place, so raw shares body's array and its offset is the difference of their
// capacities; ok is false when it does not.
func spanIn(body, raw []byte) (span, bool) {
	start := cap(body) - cap(raw)
	end := start + len(raw)
	if start < 0 || end > len(body) || !bytes.Equal(body[start:end], raw) {
		return span{}, false
	}
	return span{start, end}, true
}

// itemsOf is where each item of the array at lies in body.
func itemsOf(body []byte, at span) (items []span, ok bool) {
	err := jx.DecodeBytes(body[at.start:at.end]).Arr(func(d *jx.Decoder) error {
		raw, err := d.Raw()
		if err != nil {
			return err
		}
		item, ok := spanIn(body, raw)
		if !ok {
			return errNotInPlace
		}
		items = append(items, item)
		return nil
	})
	return items, err == nil
}

// valueOf is where the value of key lies in the object at, when the object holds key once.
func valueOf(body []byte, at span, key string) (value span, ok bool) {
	count := 0
	err := jx.DecodeBytes(body[at.start:at.end]).ObjBytes(func(d *jx.Decoder, k []byte) error {
		raw, err := d.Raw()
		if err != nil {
			return err
		}
		if string(k) != key {
			return nil
		}
		count++
		v, ok := spanIn(body, raw)
		if !ok {
			return errNotInPlace
		}
		value = v
		return nil
	})
	return value, err == nil && count == 1
}

// errNotInPlace stops a walk whose value jx did not read in place, or a decode it cannot run.
var errNotInPlace = errors.New("value not read in place")

// cutTo is body with the array at cut to its first n items.
func cutTo(body []byte, at span, items []span, n int) []byte {
	if n == len(items) {
		return body
	}
	keep := at.start + bytes.IndexByte(body[at.start:at.end], '[') + 1
	if n > 0 {
		keep = items[n-1].end
	}
	closing := at.start + bytes.LastIndexByte(body[at.start:at.end], ']')
	return slices.Concat(body[:keep], body[closing:])
}

// joinPath is path as the key of the problem's errors: its segments joined by dots, an index as
// its number (`entries.1.name`, the path a form names its field by), a key as segment quotes it.
func joinPath(path []step) string {
	var b bytes.Buffer
	for i, s := range path {
		if i > 0 {
			b.WriteByte('.')
		}
		if s.item {
			b.WriteString(strconv.Itoa(s.index))
		} else {
			b.WriteString(segment(s.key))
		}
	}
	return b.String()
}
