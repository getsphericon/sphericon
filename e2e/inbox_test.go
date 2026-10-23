//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMail is a canned row of the fake Mailpit inbox.
type fakeMail struct {
	id, to, subject string
}

// fakeMailpit serves the Mailpit endpoints the Inbox uses from a canned inbox and
// records the ids it was asked to delete.
type fakeMailpit struct {
	mu      sync.Mutex
	mails   []fakeMail
	deleted []string

	// failSearch makes the search endpoint answer 500; hangSearch makes it hold the request.
	failSearch atomic.Bool
	hangSearch atomic.Bool
}

func (f *fakeMailpit) add(m fakeMail) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mails = append(f.mails, m)
}

func (f *fakeMailpit) deletedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func (f *fakeMailpit) rows(substr string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := []map[string]any{}
	for _, m := range f.mails {
		if strings.Contains(strings.ToLower(m.to), strings.ToLower(substr)) {
			rows = append(rows, map[string]any{
				"ID": m.id, "From": map[string]string{"Address": "news@a.test"},
				"To": []map[string]string{{"Address": m.to}}, "Subject": m.subject,
			})
		}
	}
	return rows
}

// newFakeInbox starts the fake server and returns an Inbox over it.
func newFakeInbox(t *testing.T) (*Inbox, *fakeMailpit) {
	t.Helper()
	f := &fakeMailpit{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		if f.failSearch.Load() {
			http.Error(w, "mailpit is down", http.StatusInternalServerError)
			return
		}
		if f.hangSearch.Load() {
			<-r.Context().Done()
			return
		}
		q := strings.Trim(strings.TrimPrefix(r.URL.Query().Get("query"), "to:"), `"`)
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"messages": f.rows(q)}))
	})
	mux.HandleFunc("GET /api/v1/messages", func(w http.ResponseWriter, _ *http.Request) {
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"messages": f.rows("")}))
	})
	mux.HandleFunc("DELETE /api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"IDs"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		f.deleted = append(f.deleted, body.IDs...)
		f.mu.Unlock()
		_, _ = w.Write([]byte(`"ok"`))
	})
	mux.HandleFunc("GET /api/v1/message/{id}/headers", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("GET /api/v1/message/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, row := range f.rows("") {
			if row["ID"] == r.PathValue("id") {
				assert.NoError(t, json.NewEncoder(w).Encode(row))
				return
			}
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mp := NewMailpit(srv.URL)
	mp.poll = 10 * time.Millisecond
	return &Inbox{t: t, mp: mp}, f
}

func TestNoMailListsWhatTheInboxHeld(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "1", to: "other@b.test", subject: "Wrong place"})

	err := in.noMail(t.Context(), Match{To: "want@b.test"}, time.Minute, nil)

	require.Error(t, err)
	require.ErrorContains(t, err, "want@b.test")
	require.ErrorContains(t, err, "other@b.test")
	assert.ErrorContains(t, err, "Wrong place")
}

func TestNoMailReportsAnEmptyInbox(t *testing.T) {
	t.Parallel()
	in, _ := newFakeInbox(t)

	err := in.noMail(t.Context(), Match{To: "want@b.test"}, time.Minute, nil)

	assert.ErrorContains(t, err, "the inbox is empty")
}

func TestWaitMatchesTheRecipientExactlyAndCaseInsensitively(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "1", to: "xwant@b.test", subject: "Superstring"})
	f.add(fakeMail{id: "2", to: "Want@B.test", subject: "Right"})

	msg, err := in.wait(t.Context(), Match{To: "want@b.test"}, time.Second)

	require.NoError(t, err)
	assert.Equal(t, "Right", msg.Subject)
}

func TestWaitMatchesTheSubjectExactly(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "1", to: "want@b.test", subject: "First issue"})
	f.add(fakeMail{id: "2", to: "want@b.test", subject: "Second"})

	msg, err := in.wait(t.Context(), Match{To: "want@b.test", Subject: "Second"}, time.Second)
	require.NoError(t, err)
	assert.Equal(t, "2", msg.ID)

	id, err := in.find(t.Context(), Match{To: "want@b.test", Subject: "Second issue"})
	require.NoError(t, err)
	assert.Empty(t, id, "a different subject is not a match")
}

func TestInboxDeletesEveryObservedMessageOnCleanup(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "1", to: "a@b.test", subject: "waited"})
	f.add(fakeMail{id: "2", to: "c@b.test", subject: "seen by absence only"})
	f.add(fakeMail{id: "3", to: "d@b.test", subject: "untouched"})

	_, err := in.wait(t.Context(), Match{To: "a@b.test"}, time.Second)
	require.NoError(t, err)
	// An absence check that finds a match records it too (the check itself then fails).
	seen, err := in.seen(Match{To: "c@b.test"})
	require.NoError(t, err)
	assert.True(t, seen)

	in.cleanup()

	assert.ElementsMatch(t, []string{"1", "2"}, f.deletedIDs())
}

// recordingTB lets a test observe a failing assertion: FailNow records and ends the
// goroutine it runs in, the way a real failure ends the test.
type recordingTB struct {
	testing.TB

	mu     sync.Mutex
	failed bool
	logs   []string
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = true
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *recordingTB) FailNow() {
	r.mu.Lock()
	r.failed = true
	r.mu.Unlock()
	runtime.Goexit()
}

func (r *recordingTB) report() (failed bool, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed, strings.Join(r.logs, "\n")
}

// runFailing runs fn on its own goroutine and reports whether it failed the test.
func runFailing(rec *recordingTB, fn func()) (failed bool, message string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	<-done
	return rec.report()
}

func TestRequireNoneFailsWhenTheInboxCannotBeChecked(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.failSearch.Store(true)
	rec := &recordingTB{TB: t}
	in.t = rec

	failed, logs := runFailing(rec, func() {
		in.RequireNone(Match{To: "want@b.test"}, 100*time.Millisecond)
	})

	assert.True(t, failed, "an unreachable Mailpit is not absence")
	assert.Contains(t, logs, "mailpit is down")
}

func TestRequireNoneFailsOnAMatchAndTheObservedMessageIsStillDeleted(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "7", to: "want@b.test", subject: "Unexpected"})
	f.add(fakeMail{id: "8", to: "other@b.test", subject: "Untouched"})
	rec := &recordingTB{TB: t}
	in.t = rec

	failed, logs := runFailing(rec, func() {
		in.RequireNone(Match{To: "want@b.test"}, 200*time.Millisecond)
	})
	in.cleanup()

	assert.True(t, failed)
	assert.Contains(t, logs, "unexpected email")
	assert.Equal(t, []string{"7"}, f.deletedIDs())
}

func TestRememberRecordsAnIDOnce(t *testing.T) {
	t.Parallel()
	in, f := newFakeInbox(t)
	f.add(fakeMail{id: "1", to: "a@b.test", subject: "again"})

	for range 3 {
		_, err := in.wait(t.Context(), Match{To: "a@b.test"}, time.Second)
		require.NoError(t, err)
	}
	in.cleanup()

	assert.Equal(t, []string{"1"}, f.deletedIDs())
}
