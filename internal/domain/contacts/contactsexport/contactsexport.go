// Package contactsexport is the CSV export of a Segment's Contacts, or of all of them, with each
// Channel's state (ADR 0138): a Member asks for it with the columns chosen and gets a one-time link
// (downloadlinks); the browser follows it, and the file is written as it is read, a page of
// Contacts at a time through the Segment cursor (segments.Contacts) and the Channel-state read
// (eligibility.ChannelStates), with no job and no file kept. Its headers are the CSV import's
// mapping targets, so a Workspace's export imports into another one by itself.
package contactsexport

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/identity/downloadlinks"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// pageSize is how many Contacts one page of the file reads: the Channel-state read binds a few
// parameters per Contact, so a page stays in the low thousands.
const pageSize = 500

var (
	// ErrSegmentNotFound means the export names a Segment the Workspace does not have.
	ErrSegmentNotFound = validation.NotFound("contactsexport: segment not found", i18n.ErrorsSegmentNotFound)
	// ErrNoColumns means the export names no column it knows.
	ErrNoColumns = validation.Refuse("contactsexport: no column to export", i18n.ErrorsContactExportNoColumns)
	// ErrInvalidCondition means the Segment's stored Condition no longer compiles.
	ErrInvalidCondition = validation.Refuse("contactsexport: the segment's condition no longer fits the contact schema", i18n.ErrorsSegmentConditionInvalid)
)

// The keys of a CSV export's Audit diff: the columns chosen and the rows the file holds as its
// stream starts.
const (
	DiffColumns = "columns"
	DiffRows    = "rows"
)

// Module asks for and opens the export.
type Module struct {
	bus         *events.Bus
	segments    *segments.Module
	eligibility *eligibility.Module
	links       *downloadlinks.Module
	now         func() time.Time
}

// New builds the export over the modules it reads through and the Instance clock, which names the
// file by its day.
func New(bus *events.Bus, segs *segments.Module, elig *eligibility.Module, links *downloadlinks.Module, now func() time.Time) *Module {
	return &Module{bus: bus, segments: segs, eligibility: elig, links: links, now: now}
}

// LinkInput is an export to ask for: a Segment's Contacts (nil for all of them), the columns in
// the order the file has them, and the session token of the request, which alone follows the link.
type LinkInput struct {
	SegmentID *int64
	Columns   []Column
	Session   string
}

// payload is what a link of the export reads.
type payload struct {
	SegmentID *int64   `json:"segmentId,omitempty"`
	Columns   []Column `json:"columns"`
}

// Link is the one-time link of an export: its Workspace's public id, the token it carries and
// when it lapses.
type Link struct {
	Workspace string
	Token     string
	ExpiresAt time.Time
}

// Link checks the export and issues its one-time link: the Segment exists, and the columns are
// known, each kept once in its first place; no column is ErrNoColumns.
func (m *Module) Link(ctx context.Context, s *ent.Scoped, in LinkInput) (Link, error) {
	cols, err := normalise(in.Columns)
	if err != nil {
		return Link{}, err
	}
	if in.SegmentID != nil {
		if _, err := m.segments.Get(ctx, s, *in.SegmentID); err != nil {
			return Link{}, refusal(err)
		}
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("read workspace: %w", err)
	}
	link, err := m.links.Issue(ctx, s, downloadlinks.IssueInput{
		Purpose: downloadlinks.PurposeContactsExport,
		Session: in.Session,
		Payload: payload{SegmentID: in.SegmentID, Columns: cols},
	})
	if err != nil {
		return Link{}, err
	}
	return Link{Workspace: ws.PublicID, Token: link.Token, ExpiresAt: link.ExpiresAt}, nil
}

// normalise keeps the known columns once each, in their first place.
func normalise(cols []Column) ([]Column, error) {
	seen := map[Column]bool{}
	var out []Column
	for _, c := range cols {
		if seen[c] || !c.known() {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, ErrNoColumns
	}
	return out, nil
}

// OpenInput is a link followed: its token and the session token of the request that follows it.
type OpenInput struct {
	Token   string
	Session string
}

// Download is the export as the driver streams it: the name to save it under and its chunks.
type Download struct {
	Filename string
	// Chunks yields the file in order, a page of Contacts a chunk, and ends with the error that
	// stopped it, if any. They are read as they are asked for, with the context of the request.
	Chunks iter.Seq2[[]byte, error]
}

// Open follows the link (downloadlinks.ErrLinkInvalid for any that does not stand) and records the
// export as a `contact.export` Audit entry of the scope's actor, naming the Segment, the columns
// and the rows the file holds as it starts, all in one transaction and before any row leaves: a
// failure leaves the link standing and nothing exported. The file is read afterwards, as it is
// streamed.
func (m *Module) Open(ctx context.Context, s *ent.Scoped, in OpenInput) (Download, error) {
	var p payload
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		err := m.links.Follow(ctx, ts, downloadlinks.FollowInput{
			Purpose: downloadlinks.PurposeContactsExport, Token: in.Token, Session: in.Session,
		}, &p)
		if err != nil {
			return err
		}
		entry, err := m.auditEntry(ctx, ts, p)
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, entry)
	})
	if err != nil {
		return Download{}, err
	}
	return Download{Filename: m.filename(p), Chunks: m.chunks(ctx, s, p)}, nil
}

// auditEntry is the Audit entry of the export p: its Segment as the target (the Contacts as a
// whole without one), its columns and how many rows the file holds as its stream starts.
func (m *Module) auditEntry(ctx context.Context, s *ent.Scoped, p payload) (*events.AuditEntry, error) {
	entry := &events.AuditEntry{
		WorkspaceID: s.WorkspaceID(),
		Actor:       s.Actor(),
		Action:      events.ActionContactExport,
		TargetType:  contact.Label,
	}
	var counts segments.Counts
	var err error
	if p.SegmentID != nil {
		seg, getErr := m.segments.Get(ctx, s, *p.SegmentID)
		if getErr != nil {
			return nil, refusal(getErr)
		}
		entry.TargetType, entry.TargetID, entry.TargetName = segment.Label, publicid.Encode(seg.ID), seg.Name
		counts, err = m.segments.Count(ctx, s, seg.ID)
	} else {
		counts, err = m.segments.Preview(ctx, s, segments.PreviewInput{})
	}
	if err != nil {
		return nil, refusal(err)
	}
	entry.Diff = map[string]any{DiffColumns: p.Columns, DiffRows: counts.Count}
	return entry, nil
}

// refusal is the export's own refusal for a Segments one, any other error as it is.
func refusal(err error) error {
	switch {
	case errors.Is(err, segments.ErrNotFound):
		return ErrSegmentNotFound
	case errors.Is(err, segments.ErrInvalidCondition):
		return ErrInvalidCondition.With(err.Error())
	default:
		return err
	}
}

// filename names the file by what it holds and the day it was taken.
func (m *Module) filename(p payload) string {
	day := m.now().UTC().Format(time.DateOnly)
	if p.SegmentID != nil {
		return "contacts-segment-" + publicid.Encode(*p.SegmentID) + "-" + day + ".csv"
	}
	return "contacts-" + day + ".csv"
}

// chunks yields the file of the export p: the header, then a page of Contacts at a time, read
// through the Segment cursor in ascending id, each with its Channel states when a channel column
// is chosen.
func (m *Module) chunks(ctx context.Context, s *ent.Scoped, p payload) iter.Seq2[[]byte, error] {
	return csvfile.Chunks(func(yield func([][]string, error) bool) {
		layout, err := newLayout(ctx, s, p.Columns)
		if err != nil {
			yield(nil, fmt.Errorf("read the export's columns: %w", err))
			return
		}
		if !yield([][]string{layout.header()}, nil) {
			return
		}
		var after int64
		for {
			page, err := m.segments.Contacts(ctx, s, segments.ContactsInput{SegmentID: p.SegmentID, After: after, Limit: pageSize})
			if err != nil {
				yield(nil, refusal(err))
				return
			}
			var states map[int64]eligibility.ContactStates
			if layout.needsStates() {
				if states, err = m.eligibility.ChannelStates(ctx, s, page.Items); err != nil {
					yield(nil, fmt.Errorf("read channel states: %w", err))
					return
				}
			}
			rows := make([][]string, 0, len(page.Items))
			for _, c := range page.Items {
				rows = append(rows, layout.row(c, states[c.ID]))
			}
			if !yield(rows, nil) || page.Next == 0 {
				return
			}
			after = page.Next
		}
	})
}
