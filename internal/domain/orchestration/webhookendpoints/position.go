package webhookendpoints

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/ent/webhookendpointhealth"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// Only a lost conditional write is retryable here. A rejected trigger or a publishing
// error that wraps ErrInvalidTransition must still leave the transaction as an error.
var errHealthConflict = fmt.Errorf("%w: webhook health changed during the write", lifecycle.ErrInvalidTransition)

func generation(e *ent.WebhookEndpoint) int64 {
	if e.HealthGeneration == nil {
		return 0
	}
	return *e.HealthGeneration
}

// atHealth fences the endpoint's row as it was loaded in this transaction: the manual-reset
// generation and the disable moment. The failure run beside it (ADR 0168) has its own fence (atRun).
func atHealth(e *ent.WebhookEndpoint) []predicate.WebhookEndpoint {
	at := []predicate.WebhookEndpoint{webhookendpoint.UpdatedAtEQ(e.UpdatedAt)}
	if e.HealthGeneration == nil {
		at = append(at, webhookendpoint.HealthGenerationIsNil())
	} else {
		at = append(at, webhookendpoint.HealthGenerationEQ(*e.HealthGeneration))
	}
	if e.DisabledAt == nil {
		at = append(at, webhookendpoint.DisabledAtIsNil())
	} else {
		at = append(at, webhookendpoint.DisabledAtEQ(*e.DisabledAt))
	}
	return at
}

// HealthOf is the current failure run of e (ADR 0168), read through its health edge, which every
// read that shows or judges it loads. Every endpoint has the row, made with it: an edge not loaded,
// or loaded empty, is a reader's bug, and reading it as healthy would never disable a failing
// endpoint, so it panics.
func HealthOf(e *ent.WebhookEndpoint) *ent.WebhookEndpointHealth {
	health, err := e.Edges.HealthOrErr()
	if err != nil {
		panic(fmt.Sprintf("webhookendpoints: endpoint %d read without its health: %v", e.ID, err))
	}
	return health
}

// withHealth reads the endpoint id with its health, the position a change is judged from.
func withHealth(ctx context.Context, ts *ent.Scoped, id int64) (*ent.WebhookEndpoint, error) {
	return ts.WebhookEndpoint().Query().Where(webhookendpoint.ID(id)).WithHealth().Only(ctx)
}

// atRun is the compare-and-swap of the health of e (ADR 0109): the update of its row only while it
// holds the run loaded in this transaction. The counter and the series start also protect a
// same-moment move; the last write's moment alone would not.
func atRun(ts *ent.Scoped, e *ent.WebhookEndpoint) *ent.WebhookEndpointHealthScopedUpdate {
	return ts.WebhookEndpointHealth().Update().Where(runOf(e)...)
}

// runOf matches the health row of e while it holds the run loaded with e.
func runOf(e *ent.WebhookEndpoint) []predicate.WebhookEndpointHealth {
	h := HealthOf(e)
	at := []predicate.WebhookEndpointHealth{webhookendpointhealth.ID(h.ID), webhookendpointhealth.UpdatedAtEQ(h.UpdatedAt)}
	if h.ConsecutiveFailures == nil {
		at = append(at, webhookendpointhealth.ConsecutiveFailuresIsNil())
	} else {
		at = append(at, webhookendpointhealth.ConsecutiveFailuresEQ(*h.ConsecutiveFailures))
	}
	if h.FailingSince == nil {
		at = append(at, webhookendpointhealth.FailingSinceIsNil())
	} else {
		at = append(at, webhookendpointhealth.FailingSinceEQ(*h.FailingSince))
	}
	return at
}

// deliveredRunOf is runOf for a delivery's result: it holds only while the endpoint also is as the
// delivery read it, in its reset generation and enabled. An owner's disable or enable writes the
// endpoint, not always the run, so a result that meets one misses and is judged again.
func deliveredRunOf(e *ent.WebhookEndpoint) []predicate.WebhookEndpointHealth {
	held := []predicate.WebhookEndpoint{webhookendpoint.DisabledAtIsNil()}
	if e.HealthGeneration == nil {
		held = append(held, webhookendpoint.HealthGenerationIsNil())
	} else {
		held = append(held, webhookendpoint.HealthGenerationEQ(*e.HealthGeneration))
	}
	return append(runOf(e), webhookendpointhealth.HasWebhookEndpointWith(held...))
}

// keepRun is the compare of a delivery's success that writes nothing, on a healthy endpoint: it
// holds while the run and the endpoint are still as the delivery read them, and one that moved is
// errHealthConflict.
func keepRun(ctx context.Context, ts *ent.Scoped, e *ent.WebhookEndpoint) error {
	kept, err := ts.WebhookEndpointHealth().Query().Where(deliveredRunOf(e)...).Exist(ctx)
	if err == nil && !kept {
		return errHealthConflict
	}
	return err
}

// swapRun saves u, a write of atRun: a run that moved since it was loaded is errHealthConflict, and
// the transaction is retried from a fresh read.
func swapRun(ctx context.Context, u *ent.WebhookEndpointHealthScopedUpdate) error {
	n, err := u.Save(ctx)
	if err == nil && n == 0 {
		return errHealthConflict
	}
	return err
}

// clearFailures ends the endpoint's failure run.
func clearFailures(u *ent.WebhookEndpointHealthScopedUpdate) {
	u.ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
		ClearLastErrorStatus().ClearLastFailureAt()
}

// retryHealth retries only the transaction, never its HTTP request. Another current-generation
// result must not be dropped just because it raced a result that already committed.
func retryHealth(ctx context.Context, operation func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if !errors.Is(err, errHealthConflict) {
			return err
		}
	}
}
