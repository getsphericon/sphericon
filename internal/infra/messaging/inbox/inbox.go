// Package inbox implements the Test inbox provider (ADR 0058): it delivers nothing outside
// and keeps each message it is sent as a Captured message through the Capturer the catalog
// hands its builder. It has no credentials, reports a ceiling of 200 messages per rolling 24
// hours (what it keeps) and confirms delivery when it accepts a message.
package inbox

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/textproto"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// PerDay is the Test inbox's own ceiling per rolling 24 hours, equal to what it keeps per
// Workspace by default.
const PerDay = 200

// Descriptor registers the Test inbox with the messaging catalog. It takes no config: any
// stored blob is ignored.
func Descriptor() messaging.ProviderDescriptor {
	return messaging.ProviderDescriptor{
		Provider: providers.Inbox,
		Validate: func([]byte) error { return nil },
		Build: func(_ []byte, deps messaging.Deps) (any, error) {
			return &sender{capturer: deps.Capturer}, nil
		},
	}
}

// ErrNoCapturer means a Test inbox was asked to send without a place to keep the message.
var ErrNoCapturer = errors.New("inbox: no capturer")

type sender struct {
	capturer messaging.Capturer
}

// DefaultFrom is the From of a message that names none: a Test inbox has no configured one, and
// nothing sent under it reaches anyone, so it is an address of the reserved .invalid zone.
func (*sender) DefaultFrom() (address, name string) {
	return "noreply@test-inbox.invalid", "Test inbox"
}

// DeliversOutside is false: nothing a Test inbox is sent leaves sphericon.
func (*sender) DeliversOutside() bool { return false }

// SendQuota reports the Test inbox's own ceiling: 200 per rolling 24 hours and none per second.
func (*sender) SendQuota(context.Context) (messaging.Quota, error) {
	perDay := PerDay
	return messaging.Quota{PerDay: &perDay}, nil
}

// Send keeps the message and confirms its delivery on acceptance.
func (s *sender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	if s.capturer == nil {
		return messaging.Receipt{}, ErrNoCapturer
	}
	m, err := messaging.BuildMIME(msg)
	if err != nil {
		return messaging.Receipt{}, err
	}
	var wire bytes.Buffer
	if _, err := m.WriteTo(&wire); err != nil {
		return messaging.Receipt{}, fmt.Errorf("inbox: serialize message: %w", err)
	}
	id := m.GetMessageID()
	captured := messaging.Captured{
		Message:   msg,
		MessageID: id,
		Headers:   headersOf(wire.Bytes()),
		Links:     linksOf(msg.HTML),
	}
	if err := s.capturer.Capture(ctx, captured); err != nil {
		return messaging.Receipt{}, fmt.Errorf("inbox: capture message: %w", err)
	}
	return messaging.Receipt{MessageID: id, DeliveryConfirmed: true}, nil
}

// headersOf reads the header block of a serialized message, in order and with duplicates, folded
// lines joined and encoded-words (RFC 2047, a non-ASCII Subject) decoded; a value whose charset
// the decoder does not know is kept as it is.
func headersOf(wire []byte) []messaging.Header {
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(wire)))
	var dec mime.WordDecoder
	var out []messaging.Header
	for {
		line, err := r.ReadContinuedLine()
		// The blank line ends the header block; a read error can only be the end of a message
		// that has no body, which ends it too.
		if err != nil || line == "" {
			return out
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if decoded, err := dec.DecodeHeader(value); err == nil {
			value = decoded
		}
		out = append(out, messaging.Header{Name: name, Value: value})
	}
}

// linksOf is the distinct URLs of the links of an HTML part, in order of first appearance.
func linksOf(body string) []string {
	out := []string{}
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := z.Token()
			if tag.Data != "a" && tag.Data != "area" {
				continue
			}
			for _, a := range tag.Attr {
				if a.Key == "href" && a.Val != "" && !slices.Contains(out, a.Val) {
					out = append(out, a.Val)
				}
			}
		}
	}
}
