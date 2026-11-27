package contactimports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportchunk"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ReportKept is how long the refused rows' report is kept after its run ends when no one
// downloads it (ADR 0138).
const ReportKept = 7 * 24 * time.Hour

// RunAttempts is how many times an import's job is tried before the import fails: each attempt
// resumes from the checkpoint the one before it kept.
const RunAttempts = 5

// Refusals of a run.
var (
	// ErrAlreadyRunning means another import of the Workspace is running: one runs at a time.
	ErrAlreadyRunning = validation.Conflict("contactimports: another import of the workspace is running", i18n.ErrorsContactImportRunning)
	// ErrNotRunning means a cancel of an import that is not running, or that was asked to stop
	// already.
	ErrNotRunning = validation.Conflict("contactimports: the import is not running", i18n.ErrorsContactImportNotRunning)
)

// RunArgs is the job of an import's run.
type RunArgs struct {
	jobkind.Workspace

	ImportID int64 `json:"import_id"`
}

func (RunArgs) Kind() string { return "run_contact_import" }

// InsertOpts tries an import's job RunAttempts times: a try that fails is resumed from the
// import's checkpoint by the next, and the last one's failure fails the import.
func (RunArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: RunAttempts} }

// Run runs the ready import id as a Member of role: it is `running`, its start stamped and the
// Workspace's newest Contact kept as its checkpoint's floor, the Custom fields its mapping creates
// created (which needs custom_fields:manage of role, and keys the Workspace has not taken since the
// mapping was saved), and its job queued, in one transaction. An import not ready (no mapping
// saved, running or ended) is lifecycle's ErrInvalidTransition; another import of the Workspace
// running is ErrAlreadyRunning.
func (m *Module) Run(ctx context.Context, s *ent.Scoped, id int64, role membership.Role) (View, error) {
	imp, err := load(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	floor, err := m.contacts.Newest(ctx, s)
	if err != nil {
		return View{}, err
	}
	err = m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		err := constraints.Write(ctx, ts, func() error {
			return m.machine(ts, imp.ID, func(u *ent.ContactImportScopedUpdate) {
				u.SetStartedAt(m.now()).SetCheckpointContactFloor(floor)
			}).FireCtx(ctx, triggerRun)
		})
		if err != nil {
			return err
		}
		if err := m.createNewCustomFields(ctx, ts, role, imp.Mapping); err != nil {
			return err
		}
		return q.Enqueue(ctx, RunArgs{WorkspaceID: ts.WorkspaceID(), ImportID: imp.ID}, nil)
	})
	if err != nil {
		return View{}, err
	}
	return m.Get(ctx, s, id)
}

// Cancel asks the running import id to stop: its job stops after the batch it is writing, and the
// import ends `cancelled`, keeping what it wrote and its report. An import not running, or asked
// to stop already, is ErrNotRunning.
func (m *Module) Cancel(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	imp, err := load(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	err = lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := s.ContactImport().Update().
			Where(contactimport.ID(imp.ID), contactimport.StateEQ(contactimport.StateRunning), contactimport.CancelRequestedAtIsNil()).
			SetCancelRequestedAt(m.now()).
			Save(ctx)
		return n == 1, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return View{}, ErrNotRunning
	}
	if err != nil {
		return View{}, err
	}
	return m.Get(ctx, s, id)
}

// WorkInput is one try of an import's job: the import, whether it is the job's last try (its
// failure fails the import) and when the try yields (zero: never), so that no try outlives its
// job's timeout.
type WorkInput struct {
	ImportID    int64
	LastAttempt bool
	Until       time.Time
}

// Outcome is how a try of an import's job left it.
type Outcome int

const (
	// Ended: the import is no longer running (or never was): there is nothing left to try.
	Ended Outcome = iota
	// Paused: the try reached its Until with rows left; the next try resumes from the checkpoint.
	Paused
)

// Work is a try of the run of the import, its job: it reads the file from its checkpoint, a row
// at a time, into Contact intake rows (reading.row) and writes them in batches of the intake's
// size through the intake's entry for an import (contacts.Module.Import), acting as the Member who
// runs it, under the job's history mark (events.Import, ADR 0138). Before each batch it reads the
// import again: a Member asked it to stop, or its Member left the Workspace or lost
// contacts:import, and it ends `cancelled`. After each batch it keeps the rows it did not write
// whole as its report, and its checkpoint and counts; past in.Until it pauses. At the end it is
// `done`. An ending deletes the file and, while its Member is in the Workspace, records the one
// contact.import Audit entry, in one transaction. An import not running (a job queued twice) is
// left as it is. A row the intake refuses is reported; any other error of a row ends the try with
// it, and on the last try, or at once for a refusal, which no try passes, the import ends `failed`
// with what it wrote kept; a try interrupted (its
// context cancelled, as a shutdown does) is no failure: the next resumes it.
func (m *Module) Work(ctx context.Context, s *ent.Scoped, in WorkInput) (Outcome, error) {
	out, err := m.work(ctx, s, in)
	// A refusal is not tried again (the job runner ends a refused job), so it is as final as the
	// last try's failure.
	_, refused := validation.Shown(err)
	if err == nil || (!in.LastAttempt && !refused) || errors.Is(ctx.Err(), context.Canceled) {
		return out, err
	}
	// A try that ran out of time fails the import as any other error does.
	return Ended, errors.Join(err, m.fail(context.WithoutCancel(ctx), s, in.ImportID))
}

// work is Work's try, without the failure of the last.
func (m *Module) work(ctx context.Context, s *ent.Scoped, in WorkInput) (Outcome, error) {
	imp, err := load(ctx, s, in.ImportID)
	if err != nil || imp.State != contactimport.StateRunning {
		return Ended, err
	}
	rd, err := readingOf(ctx, s, imp.Header, imp.Mapping)
	if err != nil {
		return Ended, err
	}
	cause := events.Import(imp.ID)
	if imp.AsNewSignUps {
		cause = events.ImportAsNew(imp.ID)
	}
	var pending []record
	// flush writes the pending records as a batch, unless the import is to stop first.
	flush := func() (bool, error) {
		run, stop, err := m.gate(ctx, s, imp.ID)
		if err != nil || stop {
			return stop, err
		}
		run.rd, run.cause = rd, cause
		return false, run.write(ctx, pending)
	}
	for rec, err := range recordsFrom(ctx, s, imp, imp.CheckpointRow, imp.CheckpointOffset) {
		if err != nil {
			return Ended, fmt.Errorf("read the file of import %d: %w", imp.ID, err)
		}
		pending = append(pending, rec)
		if len(pending) < bounds.ContactBatchMaxItems {
			continue
		}
		if stop, err := flush(); err != nil || stop {
			return Ended, err
		}
		pending = pending[:0]
		if !in.Until.IsZero() && !m.now().Before(in.Until) {
			return Paused, nil
		}
	}
	if len(pending) > 0 {
		if stop, err := flush(); err != nil || stop {
			return Ended, err
		}
	}
	run, stop, err := m.gate(ctx, s, imp.ID)
	if err != nil || stop {
		return Ended, err
	}
	return Ended, m.end(ctx, s, run.imp, triggerFinish, run.member)
}

// gate reads the running import id again before a batch: the batch's run, as its Member, from the
// import's checkpoint. When the import is to stop instead (it is not running, a Member asked it to
// stop, its Member left the Workspace or lost contacts:import) it is ended `cancelled` and stop
// is true.
func (m *Module) gate(ctx context.Context, s *ent.Scoped, id int64) (batchRun, bool, error) {
	imp, err := load(ctx, s, id)
	if err != nil || imp.State != contactimport.StateRunning {
		return batchRun{}, true, err
	}
	ms, err := m.membership(ctx, s, imp)
	if err != nil {
		return batchRun{}, true, err
	}
	var member *ent.Scoped
	if ms != nil {
		member = m.members.ScopeOf(ms)
	}
	if ms == nil || imp.CancelRequestedAt != nil || !memberships.Allows(ms.Role, memberships.PermContactsImport) {
		return batchRun{}, true, m.end(ctx, s, imp, triggerCancel, member)
	}
	return batchRun{m: m, s: s, member: member, imp: imp}, false, nil
}

// fail ends the import id `failed` after its job's last try, as its Member while the Member is in
// the Workspace; an import no longer running is left as it is.
func (m *Module) fail(ctx context.Context, s *ent.Scoped, id int64) error {
	imp, err := load(ctx, s, id)
	if err != nil || imp.State != contactimport.StateRunning {
		return err
	}
	ms, err := m.membership(ctx, s, imp)
	if err != nil {
		return err
	}
	var member *ent.Scoped
	if ms != nil {
		member = m.members.ScopeOf(ms)
	}
	return m.end(ctx, s, imp, triggerFail, member)
}

// membership is the Membership of the Member who runs imp, with its User; nil once the Member left
// the Workspace.
func (m *Module) membership(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) (*ent.Membership, error) {
	if imp.MembershipID == nil {
		return nil, nil
	}
	ms, err := s.Membership().Query().Where(membership.ID(*imp.MembershipID)).WithUser().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return ms, err
}

// end moves the running imp by t, stamping its end and its report's expiry, and deletes its
// file, in one transaction; while its Member is in the Workspace (member) it records its one
// contact.import Audit entry as member, with how it ended, what it wrote, its mapping and its
// choices.
func (m *Module) end(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport, t trigger, member *ent.Scoped) error {
	now := m.now()
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		err := m.machine(ts, imp.ID, func(u *ent.ContactImportScopedUpdate) {
			u.SetEndedAt(now).SetReportExpiresAt(now.Add(ReportKept))
		}).FireCtx(ctx, t)
		if err != nil {
			return err
		}
		if _, err := ts.ContactImportChunk().Delete().Where(contactimportchunk.ContactImportID(imp.ID)).Exec(ctx); err != nil {
			return err
		}
		if member == nil {
			return nil
		}
		done, err := ts.ContactImport().Get(ctx, imp.ID)
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ts.WorkspaceID(),
			Actor:       member.Actor(),
			Action:      events.ActionContactImport,
			TargetType:  contact.Label,
			Diff: map[string]any{
				contactimport.FieldState:        done.State,
				contactimport.FieldCreated:      done.Created,
				contactimport.FieldUpdated:      done.Updated,
				contactimport.FieldRefused:      done.Refused,
				contactimport.FieldDropped:      done.Dropped,
				contactimport.FieldMapping:      mappingSummary(done.Header, done.Mapping),
				contactimport.FieldOnlyAddNew:   done.OnlyAddNew,
				contactimport.FieldAsNewSignUps: done.AsNewSignUps,
			},
		})
	})
}

// mappingSummary is what each column of a file wrote, for the Audit entry: "<header>: <target>",
// an ignored column left out.
func mappingSummary(header []string, mapping csvimport.Mapping) []string {
	return lo.FilterMap(mapping.Columns, func(c csvimport.Column, at int) (string, bool) {
		target := ""
		switch c.Kind {
		case csvimport.KindKey, csvimport.KindCustomField:
			target = string(c.Kind) + " " + c.Key
		case csvimport.KindAttribute:
			target = string(c.Kind) + " " + c.Attribute
		case csvimport.KindNewCustomField:
			target = string(c.Kind) + " " + c.Key + " (" + c.FieldType + ")"
		case csvimport.KindTags:
			target = string(c.Kind)
		case csvimport.KindIgnore:
			return "", false
		}
		return cellAt(header, at) + ": " + target, true
	})
}

// batchRun writes the batches of one run.
type batchRun struct {
	m      *Module
	s      *ent.Scoped
	member *ent.Scoped
	rd     reading
	imp    *ent.ContactImport
	cause  events.Cause
}

// tally is what a batch wrote. floor is the checkpoint's: a Contact of the source import above it
// was made by this batch, on this try or on one cut short before its checkpoint, and counts as
// created the first time the batch names it. newest is the newest Contact the batch wrote, the
// next batch's floor.
type tally struct {
	created, updated, refused, dropped int
	report                             []*ent.ContactImportReportRowScopedCreate
	floor, newest                      int64
	seen                               map[int64]bool
}

// write writes the records of one batch: a row refused before the intake is reported, the others
// go through the intake, each refusal it gives reported and each row written without some of its
// values reported with them. Then, in one transaction, the report's rows (a row already reported
// by a batch run again stays as it was), the checkpoint after the last record and the counts.
func (b batchRun) write(ctx context.Context, recs []record) error {
	var (
		t       = tally{floor: b.imp.CheckpointContactFloor, newest: b.imp.CheckpointContactFloor, seen: map[int64]bool{}}
		rows    []contacts.IntakeInput
		written []readRecord
	)
	for _, rec := range recs {
		rr := b.rd.row(rec.cells)
		if rr.refusal != nil {
			t.refused++
			t.report = append(t.report, b.reportRow(rec, append([]csvimport.Issue{*rr.refusal}, rr.dropped...)))
			continue
		}
		rows = append(rows, rr.in)
		written = append(written, readRecord{rec: rec, read: rr})
	}
	if len(rows) > 0 {
		out, err := b.m.contacts.Import(ctx, b.member, contacts.ImportInput{Rows: rows, Cause: b.cause, OnlyAddNew: b.imp.OnlyAddNew})
		if err != nil {
			return err
		}
		for i, item := range out.Items {
			if err := t.add(b, written[i], item.Value, item.Err); err != nil {
				return err
			}
		}
	}
	last := recs[len(recs)-1]
	return b.m.bus.WithinScopedTx(ctx, b.s, func(ts *ent.Scoped, _ events.Publisher) error {
		if len(t.report) > 0 {
			err := ts.ContactImportReportRow().CreateBulk(t.report...).
				OnConflictColumns(contactimportreportrow.FieldWorkspaceID, contactimportreportrow.FieldContactImportID, contactimportreportrow.FieldRow).
				DoNothing().
				Exec(ctx)
			if err != nil {
				return err
			}
		}
		return ts.ContactImport().Update().
			Where(contactimport.ID(b.imp.ID), contactimport.StateEQ(contactimport.StateRunning)).
			SetCheckpointRow(last.row).
			SetCheckpointChunk(int(last.next / ChunkSize)).
			SetCheckpointOffset(last.next).
			SetCheckpointContactFloor(t.newest).
			AddProcessed(len(recs)).
			AddCreated(t.created).
			AddUpdated(t.updated).
			AddRefused(t.refused).
			AddDropped(t.dropped).
			Exec(ctx)
	})
}

// readRecord is a record and the row read from it.
type readRecord struct {
	rec  record
	read readRow
}

// add counts the outcome of a row the intake was given: a refusal reports the row, a written row
// counts as created (by the intake, or by a try of this batch cut short) or updated (left as it was
// when only new Contacts are added) and is reported when it dropped values or its keys named two
// Contacts. An error that is no refusal is returned.
func (t *tally) add(b batchRun, r readRecord, res contacts.Result, err error) error {
	if err != nil {
		shown, ok := validation.Shown(err)
		if !ok {
			return fmt.Errorf("import row %d: %w", r.rec.row, err)
		}
		t.refused++
		t.report = append(t.report, b.reportRow(r.rec, append([]csvimport.Issue{refusedIssue(shown)}, r.read.dropped...)))
		return nil
	}
	id := res.Contact.ID
	if res.Created || (res.Contact.Source == contact.SourceImport && id > t.floor && !t.seen[id]) {
		t.created++
	} else {
		t.updated++
	}
	t.seen[id] = true
	t.newest = max(t.newest, id)
	t.dropped += len(r.read.dropped)
	issues := r.read.dropped
	if lo.Contains(res.Warnings, contacts.WarningKeysNameTwoContacts) {
		issues = append(issues, issue(nil, csvimport.ReasonKeysNameTwoContacts, b.rd.header))
	}
	if len(issues) > 0 {
		t.report = append(t.report, b.reportRow(r.rec, issues))
	}
	return nil
}

// reportRow is the report's row of a record and its issues.
func (b batchRun) reportRow(rec record, issues []csvimport.Issue) *ent.ContactImportReportRowScopedCreate {
	return b.s.ContactImportReportRow().Create().
		SetContactImportID(b.imp.ID).
		SetRow(rec.row).
		SetCells(rec.cells).
		SetIssues(issues)
}
