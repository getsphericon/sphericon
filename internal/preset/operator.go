package preset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Open finds the Preset an operator names for `sphericon preset test`: the Preset file at arg,
// or the built-in named arg when there is no such file. A file is parsed on its own, so its
// name may collide with a built-in's and a Self-hoster can test a copy of one.
func Open(arg string) (*Preset, error) {
	data, err := os.ReadFile(arg)
	switch {
	case err == nil:
		return Parse(filepath.Base(arg), data)
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	reg, err := Load(nil)
	if err != nil {
		return nil, err
	}
	p, ok := reg.Get(arg)
	if !ok {
		return nil, fmt.Errorf("%q is neither a file nor a built-in Preset (%s)", arg, strings.Join(reg.Names(), ", "))
	}
	return p, nil
}

// WriteTests runs the cases the Preset carries and prints PASS or FAIL for each, with how a
// failed case disagreed; it fails when any case did.
func (p *Preset) WriteTests(ctx context.Context, w io.Writer) error {
	failed := 0
	results := p.RunTests(ctx)
	for _, r := range results {
		if r.Err != nil {
			failed++
			_, _ = fmt.Fprintf(w, "FAIL %s\n     %v\n", r.Name, r.Err)
			continue
		}
		_, _ = fmt.Fprintf(w, "PASS %s\n", r.Name)
	}
	if failed > 0 {
		return fmt.Errorf("preset %s: %d of %d cases failed", p.name, failed, len(results))
	}
	return nil
}

// WriteSample verifies a sample request with the Preset's scheme and the Integration's params
// at the clock now, then prints the facts it makes, one line each, or the reason it was refused
// (and returns that refusal).
func (p *Preset) WriteSample(ctx context.Context, w io.Writer, req Request, params Params, now time.Time) error {
	res, err := p.Handle(ctx, req, params, now)
	if err != nil {
		_, _ = fmt.Fprintf(w, "refused: %v\n", err)
		return err
	}
	for _, n := range res.Notices {
		_, _ = fmt.Fprintf(w, "%s %s id=%s sending_domain=%s message_id=%s reason=%q\n",
			noticeKind(n), n.Destination, n.ID, n.SendingDomain, n.MessageID, n.Reason)
	}
	for i := range res.InboundMessages {
		m := &res.InboundMessages[i]
		_, _ = fmt.Fprintf(w, "%s %s %s id=%s chat_id=%s message_id=%s text=%q payload=%q\n",
			kindInboundMessage, m.Channel, m.Destination, m.ID, m.ChatID, m.MessageID, m.Text, m.Payload)
	}
	for _, c := range res.ReachabilityChanges {
		if c.Reachable {
			_, _ = fmt.Fprintf(w, "%s %s id=%s reachable\n", kindReachabilityChange, c.Destination, c.ID)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s %s id=%s unreachable reason=%s\n", kindReachabilityChange, c.Destination, c.ID, c.Reason)
	}
	if res.Empty() {
		_, _ = fmt.Fprintln(w, "verified, no facts (an event the Preset does not handle)")
	}
	return nil
}

// noticeKind is the kind of a notice as printed: a bounce says whether it is hard or soft.
func noticeKind(n DeliveryNotice) string {
	if n.Kind != Bounce {
		return string(n.Kind)
	}
	if n.Hard {
		return string(n.Kind) + " hard"
	}
	return string(n.Kind) + " soft"
}
