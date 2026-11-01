package staff

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrInvalidFeedCursor refuses a cursor the feed never handed out.
var ErrInvalidFeedCursor = validation.UnknownReference("staff: the cursor is not one the feed returned", i18n.ErrorsFeedCursorInvalid)

// FeedEntry is a Workspace as the feed reports it: its identity and all of its owners.
type FeedEntry struct {
	Workspace *ent.Workspace
	// Owners are the Workspace's owner Users, by id.
	Owners []*ent.User
}

// WorkspaceFeed is the Workspaces created or changed after cursor, in the order of
// their change position (then id), at most limit of them, each with all of its owners.
// An empty cursor starts from the beginning. The module keeps no position: reading is
// idempotent and the caller advances by saving NextCursor, which makes delivery
// at-least-once. A Workspace changed again after it was read is listed again, with the
// owners it has by then. Like the Workspace list, it reads tenant metadata through the
// raw client (never a Workspace's content).
func (m *Module) WorkspaceFeed(ctx context.Context, cursor string, limit int) (pagination.CursorPage[FeedEntry], error) {
	if err := m.licensed(); err != nil {
		return pagination.CursorPage[FeedEntry]{}, err
	}
	position, id, err := parseFeedCursor(cursor)
	if err != nil {
		return pagination.CursorPage[FeedEntry]{}, err
	}
	rows, err := m.ent.Workspace.Query().
		Where(entworkspace.Or(
			entworkspace.FeedPositionGT(position),
			entworkspace.And(entworkspace.FeedPositionEQ(position), entworkspace.IDGT(id)),
		)).
		Order(ent.Asc(entworkspace.FieldFeedPosition), ent.Asc(entworkspace.FieldID)).
		Limit(limit + 1).
		All(ctx)
	if err != nil {
		return pagination.CursorPage[FeedEntry]{}, err
	}
	page := pagination.Cursor(rows, limit, cursor, func(w *ent.Workspace) string { return feedCursor(w.FeedPosition, w.ID) })

	ids := make([]int64, len(page.Items))
	for i, w := range page.Items {
		ids[i] = w.ID
	}
	owners, err := accounts.OwnersOf(ctx, m.ent, ids...)
	if err != nil {
		return pagination.CursorPage[FeedEntry]{}, err
	}
	byWorkspace := make(map[int64][]*ent.User, len(page.Items))
	for _, o := range owners {
		byWorkspace[o.WorkspaceID] = append(byWorkspace[o.WorkspaceID], o.Edges.User)
	}
	return pagination.MapCursor(page, func(w *ent.Workspace) FeedEntry {
		return FeedEntry{Workspace: w, Owners: slices.Clip(byWorkspace[w.ID])}
	}), nil
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
