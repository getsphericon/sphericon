package staff

import (
	"context"
	"encoding/base64"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
)

// TrackingHostChanges is a page of the Tracking domain change log.
type TrackingHostChanges struct {
	Entries []*ent.TrackingHostChange
	// NextCursor is the cursor after the last entry, or the requested one on an empty page.
	NextCursor string
	// HasMore reports whether entries follow the page.
	HasMore bool
}

// TrackingHostFeed is the changes of Tracking domain hosts logged after cursor, in log
// order, at most limit of them (ADR 0038). An entry says a host became dns_verified or
// serving, or was removed: left both states, because its record stopped resolving, its
// Sending domain was deleted or renamed. A database trigger
// writes the log, so a removal is never silence. An empty cursor starts from the
// beginning; the module keeps no position, and delivery is at-least-once like the
// Workspace feed. It reads the log through the raw client: the entries name a Workspace
// but carry no Workspace data.
func (m *Module) TrackingHostFeed(ctx context.Context, cursor string, limit int) (TrackingHostChanges, error) {
	if err := m.licensed(); err != nil {
		return TrackingHostChanges{}, err
	}
	after, err := parseChangeCursor(cursor)
	if err != nil {
		return TrackingHostChanges{}, err
	}
	if limit <= 0 {
		limit = DefaultFeedLimit
	}
	limit = min(limit, MaxFeedLimit)

	rows, err := m.ent.TrackingHostChange.Query().
		Where(trackinghostchange.IDGT(after)).
		Order(ent.Asc(trackinghostchange.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return TrackingHostChanges{}, err
	}
	page := TrackingHostChanges{NextCursor: cursor, HasMore: len(rows) > limit}
	page.Entries = rows[:min(len(rows), limit)]
	if n := len(page.Entries); n > 0 {
		page.NextCursor = changeCursor(page.Entries[n-1].ID)
	}
	return page, nil
}

func changeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
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
