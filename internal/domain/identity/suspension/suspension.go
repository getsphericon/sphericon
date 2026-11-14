// Package suspension owns what it means for a Workspace to be suspended, and the notice its Owners
// get, and the platform's other levers over a Workspace's sending that Staff set on its row: the
// abuse exemption (ADR 0026) and the Send cap (ADR 0067). Each change is audited in the
// Workspace's own Audit log. It owns the default Send cap too, the Send cap a new Workspace starts
// with (DefaultSendCap, ADR 0146), whose change the Instance audit log records.
package suspension

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrReasonRequired refuses a suspension with no reason, whoever asks for it: the owners are told
// why their sending stopped.
var ErrReasonRequired = validation.RefuseField(fieldname.Reason, "suspension: a suspension needs a reason", i18n.ErrorsSuspensionReasonRequired)

// Actor is who suspends or lifts a suspension (ADR 0026): the automated abuse detector
// (System), the staff member command (CLI) or a platform Staff member, whose real id is kept on
// the internal record while the customer sees only "Sphericon staff".
type Actor struct {
	Kind workspace.SuspendedByKind
	ID   string // a Staff member's id; empty for System and CLI
}

var (
	System = Actor{Kind: workspace.SuspendedByKindSystem}
	CLI    = Actor{Kind: workspace.SuspendedByKindCli}
)

// StaffMember is the platform Staff member with this id.
func StaffMember(id string) Actor {
	return Actor{Kind: workspace.SuspendedByKindStaff, ID: strings.TrimSpace(id)}
}

func (a Actor) validate() error {
	if err := workspace.SuspendedByKindValidator(a.Kind); err != nil {
		return fmt.Errorf("an actor is required: %w", err)
	}
	if (a.Kind == workspace.SuspendedByKindStaff) != (a.ID != "") {
		return ErrActorInvalid
	}
	return nil
}

// ErrActorInvalid is an actor that is a Staff member without an id, or another kind with one.
var ErrActorInvalid = validation.Refuse("suspension: only a Staff member actor has an id, and it must have one",
	i18n.ErrorsSuspensionActorInvalid)

// auditActor is the actor of the Audit entry. System is the system actor; the CLI is
// platform staff acting from the server, recorded as a Staff member named "cli". Customers
// see both staff member kinds only as "Sphericon staff" (ADR 0022).
func (a Actor) auditActor() events.Actor {
	switch a.Kind {
	case workspace.SuspendedByKindSystem:
		return events.Actor{Kind: events.ActorSystem}
	case workspace.SuspendedByKindCli:
		return events.Actor{Kind: events.ActorStaff, ID: "cli"}
	default:
		return events.Actor{Kind: events.ActorStaff, ID: a.ID}
	}
}

// SuspendWorkspace freezes all outbound sending for a Workspace (ADR 0007): it sets
// suspended_at together with who did it and why; a blank reason is ErrReasonRequired. The send path reads it as a reversible Hold
// (ADR 0015); login, reads and /collect are unaffected. It reports whether anything
// changed: suspending an already-suspended Workspace is a no-op, so the first actor
// and reason — the ones the owner was told about — are never overwritten. A change is
// recorded as `workspace.suspend` in the Workspace's Audit log and published as
// events.WorkspaceSuspended, which tells its owners (ADR 0105), in the same transaction.
func SuspendWorkspace(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor, reason string, at time.Time) (bool, error) {
	changed, err := suspend(ctx, bus, byKey(workspaceID), by, reason, at)
	if err != nil {
		return false, fmt.Errorf("suspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// suspend is SuspendWorkspace of the Workspace target resolves in its transaction.
func suspend(ctx context.Context, bus *events.Bus, target workspaceTarget, by Actor, reason string, at time.Time) (bool, error) {
	reason = strings.TrimSpace(reason)
	if err := by.validate(); err != nil {
		return false, err
	}
	if reason == "" {
		return false, ErrReasonRequired
	}
	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		workspaceID, err := target(ctx, tx)
		if err != nil {
			return err
		}
		moved, err := workspaces.SuspendIn(ctx, tx, workspaceID, by.Kind, nilIfEmpty(by.ID), reason, at)
		if err != nil || !moved {
			return err
		}
		changed = true
		if err := recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, events.ActionWorkspaceSuspend,
			map[string]any{diffSuspended: events.Change(false, true), workspace.FieldSuspensionReason: events.Became(reason)}); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.WorkspaceSuspended{WorkspaceID: workspaceID})
	})
	return changed, err
}

// UnsuspendWorkspace lifts a suspension, clearing its attribution, and records
// `workspace.unsuspend` for the actor `by`. It stamps cleared_at, so the abuse detector
// (ADR 0026) counts only Events after the clearance. It reports whether the Workspace was
// suspended. Held sends resume on their own: the jobs that were snoozed ask Outbound
// send again.
func UnsuspendWorkspace(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor, at time.Time) (bool, error) {
	changed, err := unsuspend(ctx, bus, byKey(workspaceID), by, at)
	if err != nil {
		return false, fmt.Errorf("unsuspend workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// unsuspend is UnsuspendWorkspace of the Workspace target resolves in its transaction.
func unsuspend(ctx context.Context, bus *events.Bus, target workspaceTarget, by Actor, at time.Time) (bool, error) {
	if err := by.validate(); err != nil {
		return false, err
	}
	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		workspaceID, err := target(ctx, tx)
		if err != nil {
			return err
		}
		moved, err := workspaces.UnsuspendIn(ctx, tx, workspaceID, at)
		if err != nil || !moved {
			return err
		}
		changed = true
		return recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, events.ActionWorkspaceUnsuspend,
			map[string]any{diffSuspended: events.Change(true, false)})
	})
	return changed, err
}

// workspaceTarget resolves, in a suspension's transaction, the key of the Workspace it changes.
type workspaceTarget func(ctx context.Context, tx *ent.Client) (int64, error)

// byKey is the Workspace with this internal key.
func byKey(workspaceID int64) workspaceTarget {
	return func(context.Context, *ent.Client) (int64, error) { return workspaceID, nil }
}

// byPublicID is the Workspace with this Workspace id (ADR 0043), the address an operator types.
func byPublicID(publicID string) workspaceTarget {
	return func(ctx context.Context, tx *ent.Client) (int64, error) {
		return workspaces.IDByPublicIDIn(ctx, tx, publicID)
	}
}

// SetAbuseExempt exempts a Workspace from the abuse detector, or polices it again
// (ADR 0026), recorded in its Audit log for the actor `by`. It reports whether the flag
// changed. Exemption never blocks a manual suspension.
func SetAbuseExempt(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor, exempt bool) (bool, error) {
	if err := by.validate(); err != nil {
		return false, fmt.Errorf("set abuse exemption: %w", err)
	}
	action := events.ActionWorkspaceAbuseUnexempt
	if exempt {
		action = events.ActionWorkspaceAbuseExempt
	}
	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := workspaces.SetAbuseExemptIn(ctx, tx, workspaceID, exempt)
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, action,
			map[string]any{workspace.FieldAbuseExempt: events.Change(!exempt, exempt)})
	})
	if err != nil {
		return false, fmt.Errorf("set abuse exemption of workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// SetSendCap sets the Workspace's Send cap to sendCap messages per rolling 24 hours, or clears it
// with nil (ADR 0067), recorded as `workspace.send_cap_change` in its Audit log for the actor `by`
// with the old and the new value. It reports whether the value changed: setting the same value or
// clearing a Workspace with none is a no-op. The Send cap is a lever short of a suspension: a spent
// one slows sending (a Deferral), it does not stop it. Whether the Instance has Send caps at all is
// the caller's to check; the Workspace's write refuses a Send cap outside the contract's bounds
// (workspaces.CheckSendCap).
func SetSendCap(ctx context.Context, bus *events.Bus, workspaceID int64, by Actor, sendCap *int) (bool, error) {
	if err := by.validate(); err != nil {
		return false, fmt.Errorf("set send cap: %w", err)
	}

	changed := false
	err := bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		w, err := tx.Workspace.Query().Where(workspace.ID(workspaceID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		diff, differs := events.ChangeOf(w.SendCap, sendCap)
		if !differs {
			return nil
		}
		if err := workspaces.SetSendCapIn(ctx, tx, workspaceID, sendCap); err != nil {
			return err
		}
		changed = true
		return recordWorkspaceAudit(ctx, tx, pub, workspaceID, by, events.ActionWorkspaceSendCapChange,
			map[string]any{workspace.FieldSendCap: diff})
	})
	if err != nil {
		return false, fmt.Errorf("set send cap of workspace %d: %w", workspaceID, err)
	}
	return changed, nil
}

// diffSuspended is the Diff key of a suspension and its lift in the Audit log: whether the Workspace
// is suspended, which no one column holds (its suspension is three).
const diffSuspended = "suspended"

// recordWorkspaceAudit publishes the Audit entry of a change to the Workspace's sending standing.
func recordWorkspaceAudit(ctx context.Context, tx *ent.Client, pub events.Publisher, workspaceID int64, by Actor, action string, diff map[string]any) error {
	ws, err := tx.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, &events.AuditEntry{
		WorkspaceID: workspaceID,
		Actor:       by.auditActor(),
		Action:      action,
		TargetType:  workspace.Label,
		TargetID:    publicid.Encode(workspaceID),
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
