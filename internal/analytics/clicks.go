package analytics

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/linkclicks"
)

// OwnerKind is what carried a clicked link, so the page can link to that report.
type OwnerKind = linkclicks.OwnerKind

const (
	OwnerBroadcast  = linkclicks.OwnerBroadcast
	OwnerAutomation = linkclicks.OwnerAutomation
)

// OwnerClicks is how often a link was clicked in the emails of one Broadcast or Automation.
type OwnerClicks struct {
	Kind   OwnerKind
	ID     int64
	Name   string
	Clicks int
}

// LinkClicks is one link with its clicks over the window and the Broadcasts and Automations
// that carried it, most clicked first.
type LinkClicks struct {
	URL    string
	Clicks int
	Owners []OwnerClicks
}

// ClicksByLink counts the human clicks of the window over the whole Workspace by link, each
// link split by the Broadcast or Automation that carried it (ADR 0053 leaves machine clicks
// out). Every click is an Event, so the counts cover the Event retention window.
func (m *Module) ClicksByLink(ctx context.Context, s *ent.Scoped, w Window) ([]LinkClicks, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	rows, err := linkclicks.ByOwner(ctx, s, linkclicks.Between{Since: days.Since, Until: days.Until})
	if err != nil {
		return nil, err
	}
	names, err := ownerNames(ctx, s, rows)
	if err != nil {
		return nil, err
	}
	links := []LinkClicks{}
	index := map[string]int{}
	for _, r := range rows {
		i, ok := index[r.URL]
		if !ok {
			i = len(links)
			index[r.URL] = i
			links = append(links, LinkClicks{URL: r.URL})
		}
		links[i].Clicks += r.Clicks
		links[i].Owners = append(links[i].Owners, OwnerClicks{Kind: r.Kind, ID: r.ID, Name: names[ownerKey{r.Kind, r.ID}], Clicks: r.Clicks})
	}
	return links, nil
}

type ownerKey struct {
	kind OwnerKind
	id   int64
}

// ownerNames reads the names of the Broadcasts and Automations the clicks name.
func ownerNames(ctx context.Context, s *ent.Scoped, rows []linkclicks.OwnerClicks) (map[ownerKey]string, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	var broadcastIDs, automationIDs []int64
	for _, r := range rows {
		if r.Kind == OwnerBroadcast {
			broadcastIDs = append(broadcastIDs, r.ID)
		} else {
			automationIDs = append(automationIDs, r.ID)
		}
	}
	names := map[ownerKey]string{}
	broadcasts, err := s.Broadcast().Query().Where(broadcast.IDIn(broadcastIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range broadcasts {
		names[ownerKey{OwnerBroadcast, b.ID}] = b.Name
	}
	automations, err := s.Automation().Query().Where(automation.IDIn(automationIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range automations {
		names[ownerKey{OwnerAutomation, a.ID}] = a.Name
	}
	return names, nil
}
