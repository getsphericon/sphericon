package analyticsreport

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
)

// NewPeople is the New people report: the Contacts created in the window, per Report day and
// by Contact source.
type NewPeople struct {
	// Total is every Contact that signed up in the window: an imported Contact is shown in its
	// series but is no sign-up of the window (ADR 0138).
	Total  int
	Series []NewPeoplePoint
}

// NewPeoplePoint is one Report day of the New people report, zero-filled across the window.
type NewPeoplePoint struct {
	Date                         string
	Manual, API, Tracker, Import int
}

// NewPeople reports the Contacts of the Workspace behind s created in the window.
func (m *UseCase) NewPeople(ctx context.Context, s *ent.Scoped, w Window) (*NewPeople, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Day    string         `sql:"day"`
		Source contact.Source `sql:"source"`
		Count  int            `sql:"count"`
	}
	err = s.Contact().Query().
		Where(contact.CreatedAtGTE(days.Since), contact.CreatedAtLT(days.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(sel.C(contact.FieldSource), sql.As(sql.Count("*"), "count")).
				AppendSelectExprAs(reportday.Label(sel.C(contact.FieldCreatedAt), days.Loc), "day").
				GroupBy("day", sel.C(contact.FieldSource))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]NewPeoplePoint, len(days.Dates))
	report := &NewPeople{}
	for _, r := range rows {
		p := byDay[r.Day]
		switch r.Source {
		case contact.SourceManual:
			p.Manual += r.Count
			report.Total += r.Count
		case contact.SourceAPI:
			p.API += r.Count
			report.Total += r.Count
		case contact.SourceTracker:
			p.Tracker += r.Count
			report.Total += r.Count
		case contact.SourceImport:
			p.Import += r.Count
		}
		byDay[r.Day] = p
	}
	report.Series = zeroFilled(days, byDay, func(p *NewPeoplePoint, day string) { p.Date = day })
	return report, nil
}
