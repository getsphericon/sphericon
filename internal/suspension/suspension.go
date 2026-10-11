package suspension

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mokevnin/sphericon/ent"
	"github.com/mokevnin/sphericon/ent/workspace"
	"github.com/mokevnin/sphericon/internal/events"
)

// Actor is who suspends or lifts a suspension (ADR 0026): the automated abuse detector
// (System), the operator command (CLI) or a platform Operator, whose real id is kept on
// the internal record while the customer sees only "sphericon staff".
type Actor struct {
	Kind workspace.SuspendedByKind
	ID   string // an Operator's id; empty for System and CLI
}

var (
	System = Actor{Kind: workspace.SuspendedByKindSystem}
	CLI    = Actor{Kind: workspace.SuspendedByKindCli}
)

// Operator is the platform Operator with this id.
func Operator(id string) Actor {
	return Actor{Kind: workspace.SuspendedByKindOperator, ID: strings.TrimSpace(id)}
}

func (a Actor) validate() error {
	if err := workspace.SuspendedByKindValidator(a.Kind); err != nil {
		return fmt.Errorf("an actor is required: %w", err)
	}
	if (a.Kind == workspace.SuspendedByKindOperator) != (a.ID != "") {
		return errors.New("only an Operator actor has an id, and it must have one")
	}
	return nil
}

// auditActor is the actor of the Audit entry. System is the system actor; the CLI is
// platform staff acting from the server, recorded as an Operator named "cli". Customers
// see both operator kinds only as "sphericon staff" (ADR 0022).
func (a Actor) auditActor() events.Actor {
	switch a.Kind {
	case workspace.SuspendedByKindSystem:
		return events.Actor{Kind: events.ActorSystem}
	case workspace.SuspendedByKindCli:
		return events.Actor{Kind: events.ActorOperator, ID: "cli"}
	default:
		return events.Actor{Kind: events.ActorOperator, ID: a.ID}
	}
}

// SuspendWorkspace freezes all outbound sending for a Workspace (ADR 0007): it sets
// suspended_at together with who did it and why. The send path reads it as a reversible Hold
// (ADR 0015); login, reads and /collect are unaffected. It reports whether anything
// changed: suspending an already-suspended Workspace is a no-op, so the first actor
// and reason — the ones the owner was told about — are never overwritten. A change is
// recorded as `workspace.suspend` in the Workspace's Audit log in the same transaction.
func SuspendWorkspace(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor, reason string) (bool, error) {
	reason = strings.TrimSpace(reason)
	if err := by.validate(); err != nil {
		return false, fmt.Errorf("suspend workspace: %w", err)
	}
	if reason == "" {
		return false, errors.New("suspend workspace: an actor and a reason are required")
	}
	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := tx.Workspace.Update().
			Where(workspace.ID(workspaceID), workspace.SuspendedAtIsNil()).
			SetSuspendedAt(time.Now()).
			SetSuspendedByKind(by.Kind).
			SetNillableSuspendedByID(nilIfEmpty(by.ID)).
			SetSuspensionReason(reason).
			Save(ctx)
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, events.ActionWorkspaceSuspend,
			map[string]any{"suspended": map[string]any{"from": false, "to": true}, "reason": map[string]any{"to": reason}})
	})
	if err != nil {
		return false, fmt.Errorf("suspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// UnsuspendWorkspace lifts a suspension, clearing its attribution, and records
// `workspace.unsuspend` for the actor `by`. It reports whether the Workspace was
// suspended. Held sends resume on their own: the jobs that were snoozed ask Outbound
// send again.
func UnsuspendWorkspace(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor) (bool, error) {
	if err := by.validate(); err != nil {
		return false, fmt.Errorf("unsuspend workspace: %w", err)
	}
	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := tx.Workspace.Update().
			Where(workspace.ID(workspaceID), workspace.SuspendedAtNotNil()).
			ClearSuspendedAt().
			ClearSuspendedByKind().
			ClearSuspendedByID().
			ClearSuspensionReason().
			Save(ctx)
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, events.ActionWorkspaceUnsuspend,
			map[string]any{"suspended": map[string]any{"from": true, "to": false}})
	})
	if err != nil {
		return false, fmt.Errorf("unsuspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// recordWorkspaceAudit publishes the Audit entry of a suspension change.
func recordWorkspaceAudit(ctx context.Context, tx *ent.Client, pub events.Publisher, workspaceID int64, by Actor, action string, diff map[string]any) error {
	ws, err := tx.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, &events.AuditEntry{
		WorkspaceID: workspaceID,
		Actor:       by.auditActor(),
		Action:      action,
		TargetType:  "workspace",
		TargetID:    strconv.FormatInt(workspaceID, 10),
		TargetName:  ws.Name,
		Diff:        diff,
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
