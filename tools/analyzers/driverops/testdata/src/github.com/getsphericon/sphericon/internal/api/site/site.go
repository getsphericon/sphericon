package site

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/plain"
	"github.com/getsphericon/sphericon/internal/kernel/scope"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
)

type Handlers struct {
	users  *users.Module
	signup *signup.UseCase
}

// One operation, with the actor and the scope read from the context, mapping and the
// infrastructure around it.
func (h *Handlers) Get(ctx context.Context) (string, error) {
	_ = scope.From(ctx)
	u, err := h.users.Get(ctx, scope.Actor(ctx))
	if err != nil {
		return "", err
	}
	_ = plain.Publish(ctx, "read")
	return users.View(u), nil
}

func (h *Handlers) Both(ctx context.Context, id int64) error { // want `rule 5: Both calls 2 operations of the core \(users.Module.Get, users.Module.Rename\)`
	u, err := h.users.Get(ctx, id)
	if err != nil {
		return err
	}
	return h.users.Rename(ctx, u.ID, "x")
}

func (h *Handlers) ModuleAndUseCase(ctx context.Context) error { // want `rule 5: ModuleAndUseCase calls 2 operations of the core \(signup.UseCase.Complete, users.Lookup\)`
	if _, err := users.Lookup(ctx, "a@b"); err != nil {
		return err
	}
	return h.signup.Complete(ctx, "a@b")
}

// One operation on each branch.
func (h *Handlers) Branches(ctx context.Context, rename bool, kind int) error {
	if rename {
		return h.users.Rename(ctx, 1, "x")
	}
	switch kind {
	case 1:
		_, err := users.Lookup(ctx, "a")
		return err
	case 2:
		return h.signup.Complete(ctx, "a")
	default:
		_, err := h.users.Get(ctx, 1)
		return err
	}
}

// An early return leaves the second operation on another path.
func (h *Handlers) EarlyReturn(ctx context.Context, quick bool) error {
	if quick {
		return h.users.Rename(ctx, 1, "x")
	}
	return h.signup.Complete(ctx, "a")
}

// A branch that falls through to a second operation composes two.
func (h *Handlers) Fallthrough(ctx context.Context, first bool) error { // want `rule 5: Fallthrough calls 2 operations of the core`
	if first {
		_ = h.users.Rename(ctx, 1, "x")
	}
	return h.signup.Complete(ctx, "a")
}

// An operation on an error path is a second operation.
func (h *Handlers) OnError(ctx context.Context) error { // want `rule 5: OnError calls 2 operations of the core`
	if _, err := h.users.Get(ctx, 1); errors.Is(err, context.Canceled) {
		return h.signup.Complete(ctx, "a")
	}
	return nil
}

// An operation in a loop runs once per item.
func (h *Handlers) Loop(ctx context.Context, ids []int64) error { // want `rule 5: Loop calls an operation of the core in a loop \(users.Module.Rename\)`
	for _, id := range ids {
		if err := h.users.Rename(ctx, id, "x"); err != nil {
			return err
		}
	}
	return nil
}

// A helper of the driver package costs its operations at the call site.
func (h *Handlers) load(ctx context.Context) (*users.User, error) {
	return h.users.Get(ctx, 1)
}

func (h *Handlers) ThroughHelper(ctx context.Context) error { // want `rule 5: ThroughHelper calls 2 operations of the core \(users.Module.Get, users.Module.Rename\)`
	u, err := h.load(ctx)
	if err != nil {
		return err
	}
	return h.users.Rename(ctx, u.ID, "x")
}

func (h *Handlers) OnlyHelper(ctx context.Context) error {
	_, err := h.load(ctx)
	return err
}

// Recursion terminates.
func (h *Handlers) recurse(ctx context.Context, n int) error {
	if n == 0 {
		return nil
	}
	return h.recurse(ctx, n-1)
}

func (h *Handlers) Recursive(ctx context.Context) error {
	return h.recurse(ctx, 3)
}

// A function literal is a driver function of its own (a CLI command's action, a consumer); the
// function that builds it does not count its calls.
func (h *Handlers) Commands() []func(context.Context) error {
	return []func(context.Context) error{
		func(ctx context.Context) error { return h.users.Rename(ctx, 1, "x") },
		func(ctx context.Context) error { return h.signup.Complete(ctx, "a") },
		func(ctx context.Context) error { // want `rule 5: a function literal calls 2 operations of the core`
			_, _ = h.users.Get(ctx, 1)
			return h.signup.Complete(ctx, "a")
		},
	}
}
