package broadcasts

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// View is a Broadcast as the surfaces show it, built once below their contracts (ADR 0111): the
// row and its delivery figures, which are derived from its recipients and their Outbound messages
// rather than stored on it (Stats). The view of one Broadcast (Get, Send) adds its Progress while it
// is sending (ADR 0023); a list leaves it out, as it costs a query per sending row. The view a save
// returns (Create, Update) adds the lint warnings of its email content (ADR 0040), advice for the
// author that never blocks the save.
type View struct {
	*ent.Broadcast

	Stats    Stats
	Progress *Progress
	Warnings []emailrender.Finding
}

// Get returns the view of one Broadcast of the Workspace, its Progress included.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	b, err := m.row(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.detailed(ctx, s, b)
}

// List returns one page of the Workspace's Broadcasts that f lets through, in order, with their
// figures: two grouped queries for the whole page, and no Progress.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, f ListFilter, order Order) (pagination.Page[View], error) {
	page, err := m.list(ctx, s, p, f, order)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	figures, err := StatsFor(ctx, s, page.Items...)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return pagination.Map(page, func(b *ent.Broadcast) View {
		return View{Broadcast: b, Stats: figures[b.ID]}
	}), nil
}

// Create makes a draft Broadcast on the Channel of its content (create) and returns its view, with
// the lint warnings of the content as written.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, f CreateInput) (View, error) {
	b, err := m.create(ctx, s, f)
	if err != nil {
		return View{}, err
	}
	var written *string
	if c, ok := f.Content.(EmailContentInput); ok {
		written = c.Body
	}
	return m.saved(ctx, s, b, written)
}

// Update applies a patch to a draft (update) and returns its view, with the lint warnings of its
// content.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, f UpdateInput) (View, error) {
	b, err := m.update(ctx, s, id, f)
	if err != nil {
		return View{}, err
	}
	var written *string
	if c, ok := f.Content.(EmailContentPatch); ok {
		written = c.Body.Ptr()
	}
	return m.saved(ctx, s, b, written)
}

// Send moves a draft or scheduled Broadcast to sending (send) and returns its view, its Progress
// included.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	b, err := m.send(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.detailed(ctx, s, b)
}

// Schedule schedules a draft or scheduled Broadcast at when (schedule) and returns its view.
func (m *Module) Schedule(ctx context.Context, s *ent.Scoped, id int64, when time.Time) (View, error) {
	b, err := m.schedule(ctx, s, id, when)
	if err != nil {
		return View{}, err
	}
	return m.figured(ctx, s, b)
}

// Unschedule returns a scheduled Broadcast to draft (unschedule) and returns its view.
func (m *Module) Unschedule(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	b, err := m.unschedule(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.figured(ctx, s, b)
}

// SetSegment points a draft at a Segment, or at all contacts (setSegment), and returns its view.
func (m *Module) SetSegment(ctx context.Context, s *ent.Scoped, id int64, segmentID *int64) (View, error) {
	b, err := m.setSegment(ctx, s, id, segmentID)
	if err != nil {
		return View{}, err
	}
	return m.figured(ctx, s, b)
}

// Archive puts a draft, sent or failed Broadcast aside (archive) and returns its view: its figures
// stay.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	b, err := m.archive(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.figured(ctx, s, b)
}

// Unarchive brings an archived Broadcast back to the lists (unarchive) and returns its view.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	b, err := m.unarchive(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.figured(ctx, s, b)
}

// figured builds the view of b with its figures.
func (m *Module) figured(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (View, error) {
	figures, err := StatsFor(ctx, s, b)
	if err != nil {
		return View{}, err
	}
	return View{Broadcast: b, Stats: figures[b.ID]}, nil
}

// detailed builds the view of the one Broadcast b with its figures and, while it is sending, its
// Progress at the Instance's now.
func (m *Module) detailed(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (View, error) {
	d, err := m.Detail(ctx, s, b, m.out.Now())
	if err != nil {
		return View{}, err
	}
	return View{Broadcast: b, Stats: d.Stats, Progress: d.Progress}, nil
}

// saved builds the view of a Broadcast a save stored, with its figures and the lint warnings of its
// content against the Workspace's Brand kit. Only email is linted: the body as the write gave it,
// before the sanitizer, so a construct the save removed is still named; otherwise the stored body
// (a document's rendered HTML, or a write of the subject alone). A message that cannot render was
// refused by the save.
func (m *Module) saved(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, written *string) (View, error) {
	v, err := m.figured(ctx, s, b)
	if err != nil || b.Channel != channels.Email {
		return v, err
	}
	brand, err := brandkits.Brands{}.Brand(ctx, s)
	if err != nil {
		return View{}, err
	}
	body := lo.FromPtr(b.Body)
	if written != nil {
		body = *written
	}
	v.Warnings = emailrender.Lint(lo.FromPtr(b.Subject), body, brand).Warnings
	return v, nil
}
