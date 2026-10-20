//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EmailTimeout bounds every wait for an email; asynchronous steps are polled, never slept.
const EmailTimeout = 60 * time.Second

// absenceWindow is how long RequireNone watches by default.
const absenceWindow = 3 * time.Second

// Match selects mail in an Inbox.
type Match struct {
	// To is the recipient, compared exactly and case-insensitively.
	To string
	// Subject is compared exactly; empty matches any subject.
	Subject string
}

// Inbox is what one Workspace's test sees of the mail catcher. It is the only way a
// scenario observes mail: one wait, one absence check. It remembers every message it
// observed and deletes exactly those when the test ends, so parallel tests sharing
// one Mailpit stay independent.
type Inbox struct {
	t  testing.TB
	mp *Mailpit

	mu  sync.Mutex
	ids []string
}

func newInbox(tb testing.TB, mp *Mailpit) *Inbox {
	tb.Helper()
	in := &Inbox{t: tb, mp: mp}
	tb.Cleanup(in.cleanup)
	return in
}

// Wait polls (bounded by EmailTimeout) until a message matching m is in the inbox and
// returns it with its headers. On a miss it fails the test, printing what the inbox held.
func (in *Inbox) Wait(m Match) Message {
	in.t.Helper()
	msg, err := in.wait(in.t.Context(), m, EmailTimeout)
	require.NoError(in.t, err)
	return msg
}

// RequireNone asserts that nothing matching m arrives within the window (default 3 s).
// Absence cannot be awaited: callers first Wait for a sibling delivery of the same
// send, so the pipeline has provably caught up, then check absence.
func (in *Inbox) RequireNone(m Match, window ...time.Duration) {
	in.t.Helper()
	w := absenceWindow
	if len(window) > 0 {
		w = window[0]
	}
	// A failing lookup fails the test: an unreachable Mailpit must not read as absence.
	// Never runs the condition on its own goroutine and returns without waiting for one still
	// in flight, so the condition must not touch in.t: the error is handed back through lookupErr.
	var (
		mu        sync.Mutex
		lookupErr error
	)
	ok := assert.Never(in.t, func() bool {
		found, err := in.seen(m)
		if err != nil {
			mu.Lock()
			lookupErr = err
			mu.Unlock()
			return true
		}
		return found
	}, w, in.mp.poll, "unexpected email %q for %s (or the inbox could not be checked)", m.Subject, m.To)
	if ok {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if lookupErr != nil {
		in.t.Errorf("could not check the inbox for absence of %q for %s: %v", m.Subject, m.To, lookupErr)
	}
	in.t.FailNow()
}

// wait is Wait with an explicit timeout, returning the diagnostic instead of failing.
func (in *Inbox) wait(ctx context.Context, m Match, timeout time.Duration) (Message, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		id, err := in.find(ctx, m)
		if err == nil && id != "" {
			in.remember(id)
			return in.mp.get(context.WithoutCancel(ctx), id)
		}
		// The wait's own deadline cutting a request short says nothing about the inbox, so
		// it keeps the earlier error; any other failure, an HTTP timeout included, is kept.
		if ownCutoff := ctx.Err() != nil && errors.Is(err, ctx.Err()); !ownCutoff {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return Message{}, in.noMail(ctx, m, timeout, lastErr)
		case <-time.After(in.mp.poll):
		}
	}
}

// noMail is the failure of a wait that found nothing: it looks at what the inbox holds instead,
// keeping the last lookup error when listing fails too.
func (in *Inbox) noMail(ctx context.Context, m Match, waited time.Duration, lastErr error) error {
	held, lerr := in.mp.list(context.WithoutCancel(ctx), 100)
	if lerr == nil && lastErr != nil {
		lerr = lastErr
	}
	return &noMailError{match: m, waited: waited, inbox: held, listErr: lerr}
}

// seen reports whether the inbox holds a message matching m, remembering it if so.
func (in *Inbox) seen(m Match) (bool, error) {
	id, err := in.find(in.t.Context(), m)
	if err != nil || id == "" {
		return false, err
	}
	in.remember(id)
	return true, nil
}

// find returns the id of a message matching m, or "" when there is none.
func (in *Inbox) find(ctx context.Context, m Match) (string, error) {
	found, err := in.mp.search(ctx, `to:"`+m.To+`"`)
	if err != nil {
		return "", err
	}
	// Search is a substring match; insist on the exact recipient.
	for _, s := range found {
		if m.Subject != "" && s.Subject != m.Subject {
			continue
		}
		for _, a := range s.To {
			if strings.EqualFold(a.Address, m.To) {
				return s.ID, nil
			}
		}
	}
	return "", nil
}

func (in *Inbox) remember(id string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if !slices.Contains(in.ids, id) {
		in.ids = append(in.ids, id)
	}
}

// cleanup deletes every observed message; only those, never the whole inbox.
func (in *Inbox) cleanup() {
	in.mu.Lock()
	ids := in.ids
	in.mu.Unlock()
	_ = in.mp.remove(context.WithoutCancel(in.t.Context()), ids...)
}

// noMailError is a wait's timeout: it lists what the inbox held instead, so a failing
// scenario shows whether mail went to the wrong address or nowhere.
type noMailError struct {
	match  Match
	waited time.Duration
	inbox  []summary
	// listErr is set when the diagnostic listing itself failed.
	listErr error
}

func (e *noMailError) Error() string {
	var b strings.Builder
	if e.match.Subject != "" {
		fmt.Fprintf(&b, "no email %q for %s within %s; ", e.match.Subject, e.match.To, e.waited)
	} else {
		fmt.Fprintf(&b, "no email for %s within %s; ", e.match.To, e.waited)
	}
	switch {
	case e.listErr != nil:
		fmt.Fprintf(&b, "could not list the inbox: %v", e.listErr)
	case len(e.inbox) == 0:
		b.WriteString("the inbox is empty")
	default:
		fmt.Fprintf(&b, "the inbox holds %d message(s):", len(e.inbox))
		for _, s := range e.inbox {
			to := make([]string, len(s.To))
			for i, a := range s.To {
				to[i] = a.Address
			}
			fmt.Fprintf(&b, "\n  - to=[%s] from=%s subject=%q", strings.Join(to, ", "), s.From.Address, s.Subject)
		}
	}
	return b.String()
}
