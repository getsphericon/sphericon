package contactimports

import (
	"context"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// trigger is a move of an import's state.
type trigger string

const (
	// triggerSaveMapping: a Member saves the mapping, which makes the import ready to run.
	triggerSaveMapping trigger = "save_mapping"
	// triggerRun: a Member runs the import, its job queued.
	triggerRun trigger = "run"
	// triggerFinish: the job has read the whole file.
	triggerFinish trigger = "finish"
	// triggerCancel: the run stops early, what it wrote kept: a Member asked it to stop, or its
	// Member left the Workspace or lost contacts:import.
	triggerCancel trigger = "cancel"
	// triggerFail: the run stops early, what it wrote kept: its job failed its last attempt.
	triggerFail trigger = "fail"
)

// machine is the machine of the import id's state (ADR 0046, ADR 0109), written through ts, the
// scope of the transaction the move is made in: a compare-and-swap on the state it leaves, which
// also writes what set sets. uploaded and ready take a mapping, ready runs, a running import
// finishes, is cancelled or fails; nothing moves an import that ended.
func (m *Module) machine(ts *ent.Scoped, id int64, set func(*ent.ContactImportScopedUpdate)) *stateless.StateMachine {
	mach := lifecycle.New(lifecycle.Storage[contactimport.State]{
		Read: func(ctx context.Context) (contactimport.State, error) {
			imp, err := ts.ContactImport().Get(ctx, id)
			if err != nil {
				return "", err
			}
			return imp.State, nil
		},
		Write: func(ctx context.Context, from, to contactimport.State) (bool, error) {
			q := ts.ContactImport().Update().
				Where(contactimport.ID(id), contactimport.StateEQ(from)).
				SetState(to)
			set(q)
			n, err := q.Save(ctx)
			return n == 1, err
		},
	})
	mach.Configure(contactimport.StateUploaded).
		Permit(triggerSaveMapping, contactimport.StateReady)
	mach.Configure(contactimport.StateReady).
		PermitReentry(triggerSaveMapping).
		Permit(triggerRun, contactimport.StateRunning)
	mach.Configure(contactimport.StateRunning).
		Permit(triggerFinish, contactimport.StateDone).
		Permit(triggerCancel, contactimport.StateCancelled).
		Permit(triggerFail, contactimport.StateFailed)
	return mach
}
