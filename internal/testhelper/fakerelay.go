package testhelper

import (
	"context"
	"io"
	"strings"
	"sync"
)

// FakeRelayRanges is the source of the iCloud Private Relay list a test scripts, so no test
// reaches Apple (machine.Source). It publishes an empty list until a test says otherwise.
type FakeRelayRanges struct {
	mu   sync.Mutex
	body string
	err  error
}

// Publish makes the next fetches return csv, one range per row, and succeed.
func (f *FakeRelayRanges) Publish(csv string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body, f.err = csv, nil
}

// Fail makes the next fetches fail with err.
func (f *FakeRelayRanges) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Fetch implements machine.Source.
func (f *FakeRelayRanges) Fetch(context.Context) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.body)), nil
}
