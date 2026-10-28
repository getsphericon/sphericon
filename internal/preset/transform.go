package preset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

const (
	// contract-copy: byte bound of a provider payload field; not in the contract
	maxIDBytes     = 255
	maxAddrBytes   = 254
	maxDomainBytes = 253
)

// compileTransform parses and compiles a transform once, at load. The compiler gets
// no environment loader, no input iterator, no module loader and no custom function,
// so a program sees only the {body, headers} it is handed (the same sandbox as
// smoothdb's jqeval wrapper, which is the model; its global config and LRU cache have
// no place for a program compiled once).
func compileTransform(program string) (*gojq.Code, error) {
	if len(program) > MaxProgramBytes {
		return nil, fmt.Errorf("transform is over %d bytes", MaxProgramBytes)
	}
	q, err := gojq.Parse(program)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	if name, found := forbiddenCall(reflect.ValueOf(q)); found {
		return nil, fmt.Errorf("transform: %s is not available to a transform", name)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	return code, nil
}

// forbiddenFuncs are the builtins a transform may not call: debug and stderr write to the
// process's stderr and now reads the clock, so a transform would not be a pure function of
// its input (a Preset's tests could not pin its result). gojq resolves a builtin before any
// custom function, so a function of the same name cannot override one: the call is
// refused at load.
var forbiddenFuncs = map[string]bool{"debug": true, "stderr": true, "now": true, "input_filename": true}

// forbiddenCall walks the parsed program for a call to a forbidden builtin.
func forbiddenCall(v reflect.Value) (string, bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return "", false
		}
		if f, ok := reflect.TypeAssert[*gojq.Func](v); ok && forbiddenFuncs[f.Name] {
			return f.Name, true
		}
		return forbiddenCall(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if name, found := forbiddenCall(v.Field(i)); found {
				return name, true
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if name, found := forbiddenCall(v.Index(i)); found {
				return name, true
			}
		}
	}
	return "", false
}

// run evaluates the transform under the timeout and validates what comes out.
func (p *Preset) run(ctx context.Context, body any, header http.Header) ([]DeliveryNotice, error) {
	ctx, cancel := context.WithTimeout(ctx, TransformTimeout)
	defer cancel()

	input := map[string]any{"body": gojqValue(body), "headers": headerMap(header)}
	iter := p.transform.RunWithContext(ctx, input)
	var out any
	count := 0
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: exceeded %s", ErrTransformFailed, TransformTimeout)
			}
			return nil, fmt.Errorf("%w: %w", ErrTransformFailed, err)
		}
		count++
		if count > 1 {
			return nil, fmt.Errorf("%w: the program produced more than one result", ErrTransformFailed)
		}
		out = v
	}
	if count == 0 {
		return nil, fmt.Errorf("%w: the program produced no result", ErrTransformFailed)
	}
	return validateNotices(out)
}

// rawNotice is the program's output element, decoded strictly: a closed shape.
type rawNotice struct {
	ID            string  `json:"id"`
	Destination   string  `json:"destination"`
	Kind          string  `json:"kind"`
	Hard          *bool   `json:"hard"`
	SendingDomain *string `json:"sending_domain"`
	MessageID     *string `json:"message_id"`
	Reason        *string `json:"reason"`
}

// validateNotices never trusts the program's shape: a list of at most MaxNotices
// objects, each a closed kind, a parsed address, bounded strings.
func validateNotices(out any) ([]DeliveryNotice, error) {
	list, ok := out.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: the result is not an array", ErrTransformFailed)
	}
	if len(list) > MaxNotices {
		return nil, fmt.Errorf("%w: %d notices, the limit is %d", ErrTransformFailed, len(list), MaxNotices)
	}
	notices := make([]DeliveryNotice, 0, len(list))
	for i, item := range list {
		f, err := validateNotice(item)
		if err != nil {
			return nil, fmt.Errorf("%w: notice %d: %w", ErrTransformFailed, i, err)
		}
		notices = append(notices, f)
	}
	return notices, nil
}

func validateNotice(item any) (DeliveryNotice, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return DeliveryNotice{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r rawNotice
	if err := dec.Decode(&r); err != nil {
		return DeliveryNotice{}, err
	}

	if r.ID == "" || len(r.ID) > maxIDBytes || !utf8.ValidString(r.ID) {
		return DeliveryNotice{}, errors.New("id must be a non-empty string of at most 255 bytes")
	}
	kind := Kind(r.Kind)
	if !slices.Contains([]Kind{Bounce, Complaint, Delivery, Delay, Drop}, kind) {
		return DeliveryNotice{}, fmt.Errorf("kind %q is not bounce, complaint, delivery, delay or drop", r.Kind)
	}
	if len(r.Destination) > maxAddrBytes {
		return DeliveryNotice{}, errors.New("destination is too long")
	}
	addr, err := mail.ParseAddress(r.Destination)
	if err != nil {
		return DeliveryNotice{}, fmt.Errorf("destination is not an address: %w", err)
	}
	f := DeliveryNotice{
		ID:          r.ID,
		Destination: strings.ToLower(strings.TrimSpace(addr.Address)),
		Kind:        kind,
		Hard:        r.Hard != nil && *r.Hard,
	}
	if r.SendingDomain != nil && *r.SendingDomain != "" {
		domain := strings.ToLower(strings.TrimSpace(*r.SendingDomain))
		if len(domain) > maxDomainBytes || !validDomain(domain) {
			return DeliveryNotice{}, errors.New("sending_domain is not a domain")
		}
		f.SendingDomain = domain
	}
	if r.MessageID != nil && *r.MessageID != "" {
		if len(*r.MessageID) > maxIDBytes || !utf8.ValidString(*r.MessageID) {
			return DeliveryNotice{}, errors.New("message_id must be a string of at most 255 bytes")
		}
		f.MessageID = *r.MessageID
	}
	if r.Reason != nil {
		if !utf8.ValidString(*r.Reason) {
			return DeliveryNotice{}, errors.New("reason is not valid UTF-8")
		}
		f.Reason = truncateChars(strings.TrimSpace(*r.Reason), MaxReasonChars)
	}
	return f, nil
}

// truncateChars cuts s to at most n characters.
func truncateChars(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func validDomain(d string) bool {
	if d == "" {
		return false
	}
	for _, c := range d {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// gojqValue converts a body decoded with json.Number to the numbers gojq accepts,
// keeping integers exact (int or *big.Int, else float64).
func gojqValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			if i, err := strconv.Atoi(s); err == nil {
				return i
			}
			if b, ok := new(big.Int).SetString(s, 10); ok {
				return b
			}
		}
		f, err := x.Float64()
		if err != nil {
			return s
		}
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = gojqValue(e)
		}
	case []any:
		for i, e := range x {
			x[i] = gojqValue(e)
		}
	}
	return v
}
