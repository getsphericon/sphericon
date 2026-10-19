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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
)

const (
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
		if f, ok := v.Interface().(*gojq.Func); ok && forbiddenFuncs[f.Name] {
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
func (p *Preset) run(ctx context.Context, body any, header http.Header) ([]DeliveryFailure, error) {
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
		if count++; count > 1 {
			return nil, fmt.Errorf("%w: the program produced more than one result", ErrTransformFailed)
		}
		out = v
	}
	if count == 0 {
		return nil, fmt.Errorf("%w: the program produced no result", ErrTransformFailed)
	}
	return validateFailures(out)
}

// rawFailure is the program's output element, decoded strictly: a closed shape.
type rawFailure struct {
	ID            string  `json:"id"`
	Destination   string  `json:"destination"`
	Kind          string  `json:"kind"`
	Permanent     *bool   `json:"permanent"`
	SendingDomain *string `json:"sending_domain"`
}

// validateFailures never trusts the program's shape: a list of at most MaxFailures
// objects, each a closed kind, a parsed address, bounded strings.
func validateFailures(out any) ([]DeliveryFailure, error) {
	list, ok := out.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: the result is not an array", ErrTransformFailed)
	}
	if len(list) > MaxFailures {
		return nil, fmt.Errorf("%w: %d failures, the limit is %d", ErrTransformFailed, len(list), MaxFailures)
	}
	failures := make([]DeliveryFailure, 0, len(list))
	for i, item := range list {
		f, err := validateFailure(item)
		if err != nil {
			return nil, fmt.Errorf("%w: failure %d: %w", ErrTransformFailed, i, err)
		}
		failures = append(failures, f)
	}
	return failures, nil
}

func validateFailure(item any) (DeliveryFailure, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return DeliveryFailure{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r rawFailure
	if err := dec.Decode(&r); err != nil {
		return DeliveryFailure{}, err
	}

	if r.ID == "" || len(r.ID) > maxIDBytes || !utf8.ValidString(r.ID) {
		return DeliveryFailure{}, errors.New("id must be a non-empty string of at most 255 bytes")
	}
	kind := Kind(r.Kind)
	if kind != Bounce && kind != Complaint {
		return DeliveryFailure{}, fmt.Errorf("kind %q is neither bounce nor complaint", r.Kind)
	}
	if len(r.Destination) > maxAddrBytes {
		return DeliveryFailure{}, errors.New("destination is too long")
	}
	addr, err := mail.ParseAddress(r.Destination)
	if err != nil {
		return DeliveryFailure{}, fmt.Errorf("destination is not an address: %w", err)
	}
	f := DeliveryFailure{
		ID:          r.ID,
		Destination: strings.ToLower(strings.TrimSpace(addr.Address)),
		Kind:        kind,
		Permanent:   r.Permanent != nil && *r.Permanent,
	}
	if r.SendingDomain != nil && *r.SendingDomain != "" {
		domain := strings.ToLower(strings.TrimSpace(*r.SendingDomain))
		if len(domain) > maxDomainBytes || !validDomain(domain) {
			return DeliveryFailure{}, errors.New("sending_domain is not a domain")
		}
		f.SendingDomain = domain
	}
	return f, nil
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
