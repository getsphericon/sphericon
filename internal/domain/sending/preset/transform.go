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
	"github.com/samber/lo"
)

const (
	// Byte bound of a provider payload field; not in the contract.
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
func (p *Preset) run(ctx context.Context, body any, header http.Header) (Result, error) {
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
				return Result{}, fmt.Errorf("%w: exceeded %s", ErrTransformFailed, TransformTimeout)
			}
			return Result{}, fmt.Errorf("%w: %w", ErrTransformFailed, err)
		}
		count++
		if count > 1 {
			return Result{}, fmt.Errorf("%w: the program produced more than one result", ErrTransformFailed)
		}
		out = v
	}
	if count == 0 {
		return Result{}, fmt.Errorf("%w: the program produced no result", ErrTransformFailed)
	}
	return validateFacts(out)
}

// validateFacts never trusts the program's shape: a list of at most MaxFacts objects, each
// of a closed kind with that kind's closed shape, bounded strings, a parsed address where
// the kind has one.
func validateFacts(out any) (Result, error) {
	list, ok := out.([]any)
	if !ok {
		return Result{}, fmt.Errorf("%w: the result is not an array", ErrTransformFailed)
	}
	if len(list) > MaxFacts {
		return Result{}, fmt.Errorf("%w: %d facts, the limit is %d", ErrTransformFailed, len(list), MaxFacts)
	}
	var res Result
	for i, item := range list {
		if err := addFact(&res, item); err != nil {
			return Result{}, fmt.Errorf("%w: fact %d: %w", ErrTransformFailed, i, err)
		}
	}
	return res, nil
}

// noticeKinds are the kinds of a Delivery notice.
var noticeKinds = []Kind{Bounce, Complaint, Delivery, Delay, Drop, Undelivered}

// addFact validates one element of the program's output by its kind and appends it to r.
func addFact(r *Result, item any) error {
	obj, ok := item.(map[string]any)
	if !ok {
		return errors.New("a fact is an object")
	}
	kind, _ := obj["kind"].(string)
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	switch {
	case kind == KindInboundMessage:
		m, err := validateInbound(raw)
		if err != nil {
			return err
		}
		r.InboundMessages = append(r.InboundMessages, m)
	case kind == KindReachabilityChange:
		c, err := validateReachability(raw)
		if err != nil {
			return err
		}
		r.ReachabilityChanges = append(r.ReachabilityChanges, c)
	case slices.Contains(noticeKinds, Kind(kind)):
		n, err := validateNotice(raw)
		if err != nil {
			return err
		}
		r.Notices = append(r.Notices, n)
	default:
		return fmt.Errorf("kind %v is not %s, %s, bounce, complaint, delivery, delay, drop or undelivered",
			obj["kind"], KindInboundMessage, KindReachabilityChange)
	}
	return nil
}

// decodeStrict decodes one fact into its closed shape: a field outside it is refused.
func decodeStrict(raw []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

// rawNotice is a Delivery notice as the program returns it.
type rawNotice struct {
	ID            string  `json:"id"`
	Channel       *string `json:"channel"`
	Destination   string  `json:"destination"`
	Kind          string  `json:"kind"`
	Hard          *bool   `json:"hard"`
	SendingDomain *string `json:"sending_domain"`
	MessageID     *string `json:"message_id"`
	Reason        *string `json:"reason"`
	Status        *string `json:"status"`
	ErrorCode     *string `json:"error_code"`
}

// emailChannel is the Channel of a Delivery notice that names none: notices were email's alone
// before the other Channels.
const emailChannel Channel = "email"

func validateNotice(raw []byte) (DeliveryNotice, error) {
	var r rawNotice
	if err := decodeStrict(raw, &r); err != nil {
		return DeliveryNotice{}, err
	}
	if err := validID(r.ID); err != nil {
		return DeliveryNotice{}, err
	}
	ch := Channel(lo.FromPtrOr(r.Channel, string(emailChannel)))
	if !slices.Contains(channels, ch) {
		return DeliveryNotice{}, fmt.Errorf("channel %q is not a Channel", ch)
	}
	if Kind(r.Kind) == Undelivered && ch == emailChannel {
		return DeliveryNotice{}, errors.New("undelivered is not a kind on email: a message that did not arrive bounced")
	}
	dest, err := noticeDestination(ch, r.Destination)
	if err != nil {
		return DeliveryNotice{}, err
	}
	f := DeliveryNotice{
		ID:          r.ID,
		Channel:     ch,
		Destination: dest,
		Kind:        Kind(r.Kind),
		Hard:        r.Hard != nil && *r.Hard,
	}
	if f.Status, err = optionalID("status", r.Status); err != nil {
		return DeliveryNotice{}, err
	}
	if f.ErrorCode, err = optionalID("error_code", r.ErrorCode); err != nil {
		return DeliveryNotice{}, err
	}
	if r.SendingDomain != nil && *r.SendingDomain != "" {
		domain := strings.ToLower(strings.TrimSpace(*r.SendingDomain))
		if len(domain) > maxDomainBytes || !validDomain(domain) {
			return DeliveryNotice{}, errors.New("sending_domain is not a domain")
		}
		f.SendingDomain = domain
	}
	if f.MessageID, err = optionalID("message_id", r.MessageID); err != nil {
		return DeliveryNotice{}, err
	}
	if r.Reason != nil {
		if !utf8.ValidString(*r.Reason) {
			return DeliveryNotice{}, errors.New("reason is not valid UTF-8")
		}
		f.Reason = truncateChars(strings.TrimSpace(*r.Reason), MaxReasonChars)
	}
	return f, nil
}

// noticeDestination checks where a notice's message went: on email a parsed, lower-cased
// address; on another Channel a Destination as an Inbound message has one.
func noticeDestination(ch Channel, d string) (string, error) {
	if ch != emailChannel {
		return validDestination(d)
	}
	if len(d) > maxAddrBytes {
		return "", errors.New("destination is too long")
	}
	addr, err := mail.ParseAddress(d)
	if err != nil {
		return "", fmt.Errorf("destination is not an address: %w", err)
	}
	return strings.ToLower(strings.TrimSpace(addr.Address)), nil
}

// rawInbound is an Inbound message as the program returns it.
type rawInbound struct {
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Channel     string  `json:"channel"`
	Destination string  `json:"destination"`
	Country     *string `json:"country"`
	ChatID      *string `json:"chat_id"`
	Text        *string `json:"text"`
	Payload     *string `json:"payload"`
	MessageID   *string `json:"message_id"`
	CallbackID  *string `json:"callback_id"`
}

func validateInbound(raw []byte) (InboundMessage, error) {
	var r rawInbound
	if err := decodeStrict(raw, &r); err != nil {
		return InboundMessage{}, err
	}
	if err := validID(r.ID); err != nil {
		return InboundMessage{}, err
	}
	if !slices.Contains(channels, Channel(r.Channel)) {
		return InboundMessage{}, fmt.Errorf("channel %q is not a Channel", r.Channel)
	}
	dest, err := validDestination(r.Destination)
	if err != nil {
		return InboundMessage{}, err
	}
	m := InboundMessage{ID: r.ID, Channel: Channel(r.Channel), Destination: dest}
	if m.Country, err = optionalID("country", r.Country); err != nil {
		return InboundMessage{}, err
	}
	if m.ChatID, err = optionalID("chat_id", r.ChatID); err != nil {
		return InboundMessage{}, err
	}
	if m.MessageID, err = optionalID("message_id", r.MessageID); err != nil {
		return InboundMessage{}, err
	}
	if m.CallbackID, err = optionalID("callback_id", r.CallbackID); err != nil {
		return InboundMessage{}, err
	}
	if m.Text, err = optionalText("text", r.Text, MaxTextChars); err != nil {
		return InboundMessage{}, err
	}
	if m.Payload, err = optionalText("payload", r.Payload, MaxPayloadChars); err != nil {
		return InboundMessage{}, err
	}
	return m, nil
}

// rawReachability is a Reachability change as the program returns it.
type rawReachability struct {
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Destination string  `json:"destination"`
	Reachable   *bool   `json:"reachable"`
	Reason      *string `json:"reason"`
}

func validateReachability(raw []byte) (ReachabilityChange, error) {
	var r rawReachability
	if err := decodeStrict(raw, &r); err != nil {
		return ReachabilityChange{}, err
	}
	if err := validID(r.ID); err != nil {
		return ReachabilityChange{}, err
	}
	dest, err := validDestination(r.Destination)
	if err != nil {
		return ReachabilityChange{}, err
	}
	if r.Reachable == nil {
		return ReachabilityChange{}, errors.New("reachable must be true or false")
	}
	c := ReachabilityChange{ID: r.ID, Destination: dest, Reachable: *r.Reachable}
	switch reason := Unreachability(lo.FromPtr(r.Reason)); {
	case c.Reachable && r.Reason != nil:
		return ReachabilityChange{}, errors.New("a reachable Destination has no reason")
	case c.Reachable:
	case reason == Blocked, reason == Deactivated:
		c.Reason = reason
	default:
		return ReachabilityChange{}, fmt.Errorf("reason %q is not blocked or deactivated", reason)
	}
	return c, nil
}

// validID checks a fact's provider event id.
func validID(id string) error {
	if id == "" || len(id) > maxIDBytes || !utf8.ValidString(id) {
		return errors.New("id must be a non-empty string of at most 255 bytes")
	}
	return nil
}

// optionalID checks an optional provider id (a message id, a chat id): "" when absent.
func optionalID(name string, v *string) (string, error) {
	if v == nil || *v == "" {
		return "", nil
	}
	if len(*v) > maxIDBytes || !utf8.ValidString(*v) {
		return "", fmt.Errorf("%s must be a string of at most 255 bytes", name)
	}
	return *v, nil
}

// validDestination checks the Destination of an Inbound message or a Reachability change: a
// trimmed, non-empty, bounded string. It is not parsed as an email address: on most Channels
// it is not one (a Telegram user id, a phone number).
func validDestination(d string) (string, error) {
	d = strings.TrimSpace(d)
	if d == "" || len(d) > maxAddrBytes || !utf8.ValidString(d) {
		return "", errors.New("destination must be a non-empty string of at most 254 bytes")
	}
	return d, nil
}

// optionalText checks free text a person wrote: valid UTF-8, cut at limit characters.
func optionalText(name string, v *string, limit int) (string, error) {
	if v == nil {
		return "", nil
	}
	if !utf8.ValidString(*v) {
		return "", fmt.Errorf("%s is not valid UTF-8", name)
	}
	return truncateChars(*v, limit), nil
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
