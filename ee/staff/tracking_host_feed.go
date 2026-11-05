package staff

import (
	"context"
	"encoding/base64"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// TrackingHostFeedEntry is one entry of the Tracking host feed, under its Workspace's id.
type TrackingHostFeedEntry struct {
	// WorkspaceID is the Workspace's id (ADR 0043), the address the service responses name.
	WorkspaceID string
	Change      *ent.TrackingHostChange
}

// TrackingHostFeed is the changes of Tracking domain hosts logged after cursor, in log
// order, at most limit of them (ADR 0038). An entry says a host became dns_verified or
// serving, or was removed: left both states, because its record stopped resolving, its
// Sending domain was deleted or renamed. An ent hook
// (internal/trackinghostlog) writes the log, so a removal is never silence. An empty cursor starts from the
// beginning; the module keeps no position, and delivery is at-least-once like the
// Workspace feed. It reads the log through the raw client: the entries name a Workspace
// but carry no Workspace data.
func (m *Module) TrackingHostFeed(ctx context.Context, cursor string, limit int) (pagination.CursorPage[TrackingHostFeedEntry], error) {
	if err := m.licensed(); err != nil {
		return pagination.CursorPage[TrackingHostFeedEntry]{}, err
	}
	after, err := parseChangeCursor(cursor)
	if err != nil {
		return pagination.CursorPage[TrackingHostFeedEntry]{}, err
	}
	rows, err := m.ent.TrackingHostChange.Query().
		Where(trackinghostchange.IDGT(after)).
		Order(ent.Asc(trackinghostchange.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return pagination.CursorPage[TrackingHostFeedEntry]{}, err
	}
	page := pagination.Cursor(rows, limit, cursor, changeCursor)
	publicIDs, err := publicIDsOf(ctx, m, page.Items, func(c *ent.TrackingHostChange) int64 { return c.WorkspaceID })
	if err != nil {
		return pagination.CursorPage[TrackingHostFeedEntry]{}, err
	}
	return pagination.MapCursor(page, func(c *ent.TrackingHostChange) TrackingHostFeedEntry {
		return TrackingHostFeedEntry{WorkspaceID: publicIDs[c.WorkspaceID], Change: c}
	}), nil
}

func changeCursor(c *ent.TrackingHostChange) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.ID, 10)))
}

// parseChangeCursor reads a cursor back; the empty one is the beginning.
func parseChangeCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidFeedCursor
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 0 {
		return 0, ErrInvalidFeedCursor
	}
	return id, nil
}
