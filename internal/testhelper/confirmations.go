package testhelper

import (
	"context"
	"sync"
)

// Confirmations stands in for the network behind a Preset's confirm action (an SNS
// SubscribeURL), so no test fetches a URL. It records what the hook asked to confirm.
type Confirmations struct {
	mu   sync.Mutex
	urls []string
	err  error
}

// Fail makes the next confirmations fail with err (nil clears it).
func (c *Confirmations) Fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

// URLs are the URLs confirmed so far, in order.
func (c *Confirmations) URLs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.urls...)
}

func (c *Confirmations) confirm(_ context.Context, url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.urls = append(c.urls, url)
	return nil
}
