package webhookendpoints

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
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

// atHealth fences both the manual-reset generation and the position loaded in this transaction.
// The counter and series start also protect a same-activity move; a state name alone would not.
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
	if e.ConsecutiveFailures == nil {
		at = append(at, webhookendpoint.ConsecutiveFailuresIsNil())
	} else {
		at = append(at, webhookendpoint.ConsecutiveFailuresEQ(*e.ConsecutiveFailures))
	}
	if e.FailingSince == nil {
		at = append(at, webhookendpoint.FailingSinceIsNil())
	} else {
		at = append(at, webhookendpoint.FailingSinceEQ(*e.FailingSince))
	}
	return at
}

func clearFailures(u *ent.WebhookEndpointScopedUpdateOne) *ent.WebhookEndpointScopedUpdateOne {
	return u.ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
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
