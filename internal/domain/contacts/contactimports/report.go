package contactimports

import (
	"context"
	"iter"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportreportrow"
	"github.com/getsphericon/sphericon/internal/domain/identity/downloadlinks"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Refusals of the refused rows' report.
var (
	// ErrReportPending means the import has not ended: its report is still growing.
	ErrReportPending = validation.Conflict("contactimports: the import has not ended", i18n.ErrorsContactImportReportPending)
	// ErrNoReport means the import's report has no row: nothing was refused or dropped, or it was
	// downloaded or expired.
	ErrNoReport = validation.NotFound("contactimports: the import has no report to download", i18n.ErrorsContactImportNoReport)
)

// reportPage is how many of a report's rows are read at a time.
const reportPage = 1000

// IssuesHeader is the header of the report's column that says why each row is in it.
const IssuesHeader = "Issues"

// ReportLink is the one-time link to an import's report: its Workspace's public id and the
// import, which the download's path names, the link's token and when it lapses.
type ReportLink struct {
	Workspace string
	ImportID  int64
	Token     string
	ExpiresAt time.Time
}

// ReportLink issues the link to the refused rows' report of the import id, in the session the
// request is made in (downloadlinks): an import that has not ended is ErrReportPending, one whose
// report has no row ErrNoReport.
func (m *Module) ReportLink(ctx context.Context, s *ent.Scoped, id int64, session string) (ReportLink, error) {
	v, err := m.Get(ctx, s, id)
	if err != nil {
		return ReportLink{}, err
	}
	if !ended(v.Import.State) {
		return ReportLink{}, ErrReportPending
	}
	if v.ReportRows == 0 {
		return ReportLink{}, ErrNoReport
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return ReportLink{}, err
	}
	link, err := m.links.Issue(ctx, s, downloadlinks.IssueInput{Purpose: downloadlinks.PurposeImportReport, Session: session, Payload: reportPayload{ImportID: id}})
	if err != nil {
		return ReportLink{}, err
	}
	return ReportLink{Workspace: ws.PublicID, ImportID: id, Token: link.Token, ExpiresAt: link.ExpiresAt}, nil
}

// reportPayload is what a report's link names: its import.
type reportPayload struct {
	ImportID int64 `json:"importId"`
}

// ErrReportLinkInvalid means a report's link that names another import than the route.
var ErrReportLinkInvalid = validation.LinkInvalid("contactimports: the link names another import's report", i18n.ErrorsDownloadLinkInvalid)

// ended reports whether an import's run is over.
func ended(state contactimport.State) bool {
	return lo.Contains([]contactimport.State{contactimport.StateDone, contactimport.StateCancelled, contactimport.StateFailed}, state)
}

// ReportInput is a followed link to an import's report: the import the route names, the link's
// token and the session it is followed in.
type ReportInput struct {
	ImportID       int64
	Token, Session string
}

// Download is a file a handler streams: its name, its chunks, and what is done once the file
// reached the client whole.
type Download struct {
	Filename string
	Chunks   iter.Seq2[[]byte, error]
	// Delivered deletes the report. The driver calls it only after the last chunk was written and
	// the response flushed without error, so a stream cut short keeps the report.
	Delivered func(context.Context) error
}

// Report follows the link of in and streams the import's refused rows' report as CSV through the
// core writer (csvfile): the file's header with an Issues column, then each reported row's
// original cells, in the file's order, with what was refused or dropped and why. The report is
// deleted by the Download's Delivered, once its driver has delivered it whole, so a stream cut
// short keeps it. A link that does not stand is downloadlinks.ErrLinkInvalid.
func (m *Module) Report(ctx context.Context, s *ent.Scoped, in ReportInput) (Download, error) {
	imp, err := load(ctx, s, in.ImportID)
	if err != nil {
		return Download{}, err
	}
	var named reportPayload
	err = m.links.Follow(ctx, s, downloadlinks.FollowInput{Purpose: downloadlinks.PurposeImportReport, Token: in.Token, Session: in.Session}, &named)
	if err != nil {
		return Download{}, err
	}
	if named.ImportID != imp.ID {
		return Download{}, ErrReportLinkInvalid
	}
	return Download{
		Filename:  "contact-import-" + publicid.Encode(imp.ID) + "-report.csv",
		Chunks:    csvfile.Chunks(m.reportPages(ctx, s, imp)),
		Delivered: func(ctx context.Context) error { return m.dropReport(ctx, s, imp.ID) },
	}, nil
}

// reportPages are the pages of imp's report as CSV records, its header first.
func (m *Module) reportPages(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) iter.Seq2[[][]string, error] {
	return func(yield func([][]string, error) bool) {
		page := [][]string{append(append([]string{}, imp.Header...), IssuesHeader)}
		after := 0
		for {
			rows, err := s.ContactImportReportRow().Query().
				Where(contactimportreportrow.ContactImportID(imp.ID), contactimportreportrow.RowGT(after)).
				Order(ent.Asc(contactimportreportrow.FieldRow)).
				Limit(reportPage).
				All(ctx)
			if err != nil {
				yield(nil, err)
				return
			}
			for _, r := range rows {
				page = append(page, append(append([]string{}, r.Cells...), issuesText(r.Issues)))
				after = r.Row
			}
			if len(page) > 0 && !yield(page, nil) {
				return
			}
			page = nil
			if len(rows) < reportPage {
				return
			}
		}
	}
}

// dropReport deletes the report of the import id, delivered whole.
func (m *Module) dropReport(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if _, err := ts.ContactImportReportRow().Delete().Where(contactimportreportrow.ContactImportID(id)).Exec(ctx); err != nil {
			return err
		}
		return ts.ContactImport().UpdateOneID(id).ClearReportExpiresAt().Exec(ctx)
	})
}

// issuesText is a report row's issues as the text of its Issues cell.
func issuesText(issues []csvimport.Issue) string {
	return strings.Join(lo.Map(issues, func(i csvimport.Issue, _ int) string { return i.Message }), "; ")
}
