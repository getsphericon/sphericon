package site

import "context"

// A test is no driver.
func (h *Handlers) twice(ctx context.Context) {
	_, _ = h.users.Get(ctx, 1)
	_, _ = h.users.Get(ctx, 2)
}
