// Package contactimports is the CSV import of Contacts (ADR 0138): a Member holding
// contacts:import uploads a file, which is kept with its job (ent.ContactImport) as ordered
// chunks; maps each column to what it writes; previews what the mapping would do; and runs it.
// The run is a river job that reads the file, turns each row into the values of a Contact intake
// row with the Contacts module's readers (contacts.ReadText and its kin), dropping and reporting
// what a reader cannot read, and writes the rows in batches through the intake's entry for an
// import (contacts.Module.Import), acting as the Member, under the job's history mark. What it
// did not write whole is kept as the refused rows' report, downloaded once through a link bound to
// the session (downloadlinks). The whole job is one contact.import Audit entry.
//
// The tables are this module's alone: contact_imports, contact_import_chunks and
// contact_import_report_rows.
package contactimports

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/domain/identity/downloadlinks"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The unique constraints of the import's tables (ADR 0106): no client can break either.
var _ = db.Declare(
	db.Fault("contactimportchunk_workspace_id_contact_import_id_seq", "an upload writes the chunks of a new import once each, numbered in order"),
	db.Fault("contactimportreportrow_workspace_id_contact_import_id_row", "a report's row is inserted ON CONFLICT (workspace_id, contact_import_id, row) DO NOTHING"),
)

// ErrNotFound means the Workspace has no such import.
var ErrNotFound = validation.NotFound("contactimports: import not found", i18n.ErrorsContactImportNotFound)

// Module is the CSV import.
type Module struct {
	bus          *events.Bus
	contacts     *contacts.Module
	members      *memberships.Module
	links        *downloadlinks.Module
	optOuts      OptOuts
	suppressions *suppressions.Module
	now          func() time.Time
}

// OptOuts records the opt-outs an import's refusal columns bring, each with its effects as any
// opt-out has them (ADR 0138): the unsubscribe use case, which sits above this module.
type OptOuts interface {
	// OptOutImported records an opt-out `via: imported`, at the time it occurred (the zero time
	// is now), and reports whether it stands: one older than the Destination's later return
	// changes nothing (ADR 0154). It is idempotent.
	OptOutImported(ctx context.Context, s *ent.Scoped, in ImportedOptOut) (bool, error)
}

// ImportedOptOut is an opt-out a row of an import records: the Channel, the Destination, the
// sending source (`everything` or `broadcasts`) and when it occurred.
type ImportedOptOut struct {
	Channel     channels.Channel
	Destination string
	Source      string
	At          time.Time
}

// New builds the module. The bus opens the transactions of an import's writes and queues its job;
// people is the Contacts module whose readers read the cells and whose intake writes the rows;
// members builds the scope of the Member a job acts as; links issues and claims the report's
// download link; optOuts records the opt-outs its rows bring; now is the Instance clock, which
// also stamps its Suppressions' module.
func New(bus *events.Bus, people *contacts.Module, members *memberships.Module, links *downloadlinks.Module, optOuts OptOuts, now func() time.Time) *Module {
	return &Module{bus: bus, contacts: people, members: members, links: links, optOuts: optOuts, suppressions: suppressions.New(now), now: now}
}

// View is an import as a Member sees it: its row, the mapping it reads its file by (the saved one,
// or before one is saved the proposal read from the file) and the rows its report holds.
type View struct {
	Import  *ent.ContactImport
	Mapping csvimport.Mapping
	// ReportRows are the rows of the refused rows' report: 0 when there is nothing to download.
	ReportRows int
}

// Get is the import id of the Workspace.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	imp, err := load(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.view(ctx, s, imp)
}

// load reads the import id of the Workspace, ErrNotFound when it has none.
func load(ctx context.Context, s *ent.Scoped, id int64) (*ent.ContactImport, error) {
	imp, err := s.ContactImport().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return imp, err
}

// view is the View of imp: its saved mapping or the proposal, and its report's rows.
func (m *Module) view(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) (View, error) {
	mapping := imp.Mapping
	if len(mapping.Columns) == 0 {
		proposal, err := m.propose(ctx, s, imp)
		if err != nil {
			return View{}, err
		}
		mapping = proposal
	}
	rows, err := s.ContactImportReportRow().Query().Where(contactimportreportrow.ContactImportID(imp.ID)).Count(ctx)
	if err != nil {
		return View{}, err
	}
	return View{Import: imp, Mapping: mapping, ReportRows: rows}, nil
}
