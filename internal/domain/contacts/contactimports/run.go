package contactimports

import (
	"context"
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
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ReportKept is how long the refused rows' report is kept after its run ends when no one
// downloads it (ADR 0138).
const ReportKept = 7 * 24 * time.Hour

// RunArgs is the job of an import's run.
type RunArgs struct {
	jobkind.Workspace

	ImportID int64 `json:"import_id"`
}

func (RunArgs) Kind() string { return "run_contact_import" }

// InsertOpts runs an import once: its job is not retried, since a run from the start again would
// write its report rows and counts twice; resuming from its checkpoint is ADR 0138's next step.
func (RunArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

// Run runs the ready import id as a Member of role: it is `running`, its start stamped, the Custom
// fields its mapping creates created (which needs custom_fields:manage of role, and keys the
// Workspace has not taken since the mapping was saved), and its job queued, in one transaction. An
// import not ready (no mapping saved, running or ended) is lifecycle's ErrInvalidTransition.
func (m *Module) Run(ctx context.Context, s *ent.Scoped, id int64, role membership.Role) (View, error) {
	imp, err := load(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	err = m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		err := m.machine(ts, imp.ID, func(u *ent.ContactImportScopedUpdate) { u.SetStartedAt(m.now()) }).
			FireCtx(ctx, triggerRun)
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

// Work is the run of the import id, its job: it reads the file from its checkpoint, a row at a
// time, into Contact intake rows (reading.row) and writes them in batches of the intake's size
// through the intake's entry for an import (contacts.Module.Import), acting as the Member who runs
// it, under the job's history mark (events.Import, ADR 0138). After each batch it keeps the rows it
// did not write whole as its report, and its checkpoint and counts. At the end it is `done`, its
// file deleted and one contact.import Audit entry recorded, in one transaction. An import not
// running (a job queued twice) is left as it is; one whose Member left the Workspace is
// `cancelled`, what it wrote kept. A row the intake refuses is reported; any other error of a row
// ends the job with it.
func (m *Module) Work(ctx context.Context, s *ent.Scoped, id int64) error {
	imp, err := load(ctx, s, id)
	if err != nil || imp.State != contactimport.StateRunning {
		return err
	}
	member, err := m.memberScope(ctx, s, imp)
	if err != nil {
		return err
	}
	if member == nil {
		return m.end(ctx, s, imp, triggerCancel, nil)
	}
	rd, err := readingOf(ctx, s, imp.Header, imp.Mapping)
	if err != nil {
		return err
	}
	cause := events.Import(imp.ID)
	if imp.AsNewSignUps {
		cause = events.ImportAsNew(imp.ID)
	}
	run := batchRun{m: m, s: s, member: member, rd: rd, imp: imp, cause: cause}
	var pending []record
	for rec, err := range recordsFrom(ctx, s, imp, imp.CheckpointRow, imp.CheckpointOffset) {
		if err != nil {
			return fmt.Errorf("read the file of import %d: %w", imp.ID, err)
		}
		pending = append(pending, rec)
		if len(pending) < bounds.ContactBatchMaxItems {
			continue
		}
		if err := run.write(ctx, pending); err != nil {
			return err
		}
		pending = pending[:0]
	}
	if len(pending) > 0 {
		if err := run.write(ctx, pending); err != nil {
			return err
		}
	}
	return m.end(ctx, s, imp, triggerFinish, member)
}

// memberScope is the scope of the Member who runs imp, acting as its User; nil once the Member
// left the Workspace.
func (m *Module) memberScope(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) (*ent.Scoped, error) {
	if imp.MembershipID == nil {
		return nil, nil
	}
	ms, err := s.Membership().Query().Where(membership.ID(*imp.MembershipID)).WithUser().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m.members.ScopeOf(ms), nil
}

// end moves the running imp by t, stamping its end and its report's expiry, and deletes its
// file, in one transaction; a run that finished records its one contact.import Audit entry as
// member, with what it wrote, its mapping and its choices.
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

// tally is what a batch wrote.
type tally struct {
	created, updated, refused, dropped int
	report                             []*ent.ContactImportReportRowScopedCreate
}

// write writes the records of one batch: a row refused before the intake is reported, the others
// go through the intake, each refusal it gives reported and each row written without some of its
// values reported with them. Then, in one transaction, the report's rows (a row already reported
// by a batch run again stays as it was), the checkpoint after the last record and the counts.
func (b batchRun) write(ctx context.Context, recs []record) error {
	var (
		t       tally
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
// counts as created or updated (left as it was when only new Contacts are added) and is reported
// when it dropped values or its keys named two Contacts. An error that is no refusal is returned.
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
	if res.Created {
		t.created++
	} else {
		t.updated++
	}
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
