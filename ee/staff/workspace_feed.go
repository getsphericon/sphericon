package staff

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
)

// ErrInvalidFeedCursor refuses a cursor the feed never handed out.
var ErrInvalidFeedCursor = errors.New("staff: the cursor is not one the workspace feed returned")

const (
	// DefaultFeedLimit is the page size of a feed read that names none.
	DefaultFeedLimit = 100
	// MaxFeedLimit is the most entries one feed page carries.
	MaxFeedLimit = 200
)

// FeedEntry is a Workspace as the feed reports it: its identity and all of its owners.
type FeedEntry struct {
	Workspace *ent.Workspace
	// Owners are the Workspace's owner Users, by id.
	Owners []*ent.User
}

// FeedPage is one page of the Workspace feed.
type FeedPage struct {
	Entries []FeedEntry
	// NextCursor is the cursor after the last entry, or the requested one on an empty page.
	NextCursor string
	// HasMore reports whether entries follow the page.
	HasMore bool
}

// WorkspaceFeed is the Workspaces created or changed after cursor, in the order of
// their change position (then id), at most limit of them, each with all of its owners.
// An empty cursor starts from the beginning. The module keeps no position: reading is
// idempotent and the caller advances by saving NextCursor, which makes delivery
// at-least-once. A Workspace changed again after it was read is listed again, with the
// owners it has by then. Like the Workspace list, it reads tenant metadata through the
// raw client (never a Workspace's content).
func (m *Module) WorkspaceFeed(ctx context.Context, cursor string, limit int) (FeedPage, error) {
	if err := m.licensed(); err != nil {
		return FeedPage{}, err
	}
	position, id, err := parseFeedCursor(cursor)
	if err != nil {
		return FeedPage{}, err
	}
	if limit <= 0 {
		limit = DefaultFeedLimit
	}
	limit = min(limit, MaxFeedLimit)

	rows, err := m.ent.Workspace.Query().
		Where(entworkspace.Or(
			entworkspace.FeedPositionGT(position),
			entworkspace.And(entworkspace.FeedPositionEQ(position), entworkspace.IDGT(id)),
		)).
		Order(ent.Asc(entworkspace.FieldFeedPosition), ent.Asc(entworkspace.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return FeedPage{}, err
	}
	page := FeedPage{NextCursor: cursor, HasMore: len(rows) > limit}
	rows = rows[:min(len(rows), limit)]

	ids := make([]int64, len(rows))
	for i, w := range rows {
		ids[i] = w.ID
	}
	owners, err := m.ent.Membership.Query().
		Where(membership.WorkspaceIDIn(ids...), membership.RoleEQ(membership.RoleOwner)).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
	if err != nil {
		return FeedPage{}, err
	}
	byWorkspace := make(map[int64][]*ent.User, len(rows))
	for _, o := range owners {
		byWorkspace[o.WorkspaceID] = append(byWorkspace[o.WorkspaceID], o.Edges.User)
	}
	for _, w := range rows {
		page.Entries = append(page.Entries, FeedEntry{Workspace: w, Owners: slices.Clip(byWorkspace[w.ID])})
	}
	if last, ok := lastOf(rows); ok {
		page.NextCursor = feedCursor(last.FeedPosition, last.ID)
	}
	return page, nil
}

func lastOf(rows []*ent.Workspace) (*ent.Workspace, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	return rows[len(rows)-1], true
}

func feedCursor(position, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d.%d", position, id)))
}

// parseFeedCursor reads a cursor back; the empty one is the beginning (position 0, id 0).
func parseFeedCursor(cursor string) (position, id int64, err error) {
	if cursor == "" {
		return 0, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0, ErrInvalidFeedCursor
	}
	p, i, ok := strings.Cut(string(raw), ".")
	if !ok {
		return 0, 0, ErrInvalidFeedCursor
	}
	position, perr := strconv.ParseInt(p, 10, 64)
	id, ierr := strconv.ParseInt(i, 10, 64)
	if perr != nil || ierr != nil || position < 0 || id < 0 {
		return 0, 0, ErrInvalidFeedCursor
	}
	return position, id, nil
}
