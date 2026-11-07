// Package ent is a stub of the generated ent package: the client, the scoped client and a builder.
package ent

import "context"

type TagCreate struct{}

func (c *TagCreate) SetName(string) *TagCreate { return c }

func (*TagCreate) Save(context.Context) error { return nil }

func (c *TagCreate) OnConflictColumns(...string) *TagCreate { return c }

func (c *TagCreate) OnConflict() *TagCreate { return c }

type TagUpdate struct{}

func (*TagUpdate) Exec(context.Context) error { return nil }

type TagQuery struct{}

func (*TagQuery) All(context.Context) ([]string, error) { return nil, nil }

type TagClient struct{}

func (*TagClient) Create() *TagCreate           { return &TagCreate{} }
func (*TagClient) UpdateOneID(int64) *TagUpdate { return &TagUpdate{} }
func (*TagClient) DeleteOneID(int64) *TagUpdate { return &TagUpdate{} }
func (*TagClient) Query() *TagQuery             { return &TagQuery{} }

type Client struct{ Tag *TagClient }

type TagScoped struct{}

func (*TagScoped) Create() *TagCreate               { return &TagCreate{} }
func (*TagScoped) Update() *TagUpdate               { return &TagUpdate{} }
func (*TagScoped) CreateBulk() *TagCreate           { return &TagCreate{} }
func (*TagScoped) Query() *TagQuery                 { return &TagQuery{} }
func (*TagScoped) Get(context.Context, int64) error { return nil }

type Tag struct{}

func (*Tag) QueryContacts() *TagQuery { return &TagQuery{} }

type Workspace struct{}

type Scoped struct{}

func (*Scoped) Tag() *TagScoped                               { return &TagScoped{} }
func (*Scoped) WorkspaceID() int64                            { return 0 }
func (*Scoped) Workspace(context.Context) (*Workspace, error) { return nil, nil }

// The types below stand in for what the ownership map is tested on: a second entity with its
// own owner (Contact), an archivable one (Segment), a Versioned definition (Automation and its
// pointers), the lifecycle's store and an entity the map does not name (Ghost).

type TagScopedDelete struct{}

func (d *TagScopedDelete) Where(...int64) *TagScopedDelete { return d }
func (*TagScopedDelete) Exec(context.Context) (int, error) { return 0, nil }

func (*TagScoped) Delete() *TagScopedDelete { return &TagScopedDelete{} }

type ContactScopedUpdateOne struct{}

func (u *ContactScopedUpdateOne) AddTagIDs(...int64) *ContactScopedUpdateOne { return u }
func (*ContactScopedUpdateOne) Exec(context.Context) error                    { return nil }

type ContactScoped struct{}

func (*ContactScoped) UpdateOneID(int64) *ContactScopedUpdateOne { return &ContactScopedUpdateOne{} }

func (*Scoped) Contact() *ContactScoped { return &ContactScoped{} }

type SegmentScoped struct{}

func (*SegmentScoped) Archive(context.Context, int64) error { return nil }
func (*SegmentScoped) Purge() *TagScopedDelete              { return &TagScopedDelete{} }

func (*Scoped) Segment() *SegmentScoped { return &SegmentScoped{} }

type AutomationPointers struct{}

func (*AutomationPointers) CompareAndSwap(context.Context, int64, int64) error { return nil }

type AutomationScoped struct{}

func (*AutomationScoped) Pointers() *AutomationPointers { return &AutomationPointers{} }

func (*Scoped) Automation() *AutomationScoped { return &AutomationScoped{} }

// VersionedStore is what the owner of a Versioned definition binds its pointers to.
type VersionedStore[O, V any] interface {
	CompareAndSwap(ctx context.Context, id, versionID int64) error
}

type GhostCreate struct{}

func (*GhostCreate) Save(context.Context) error { return nil }

type GhostClient struct{}

func (*GhostClient) Create() *GhostCreate { return &GhostCreate{} }

func (*Client) Ghost() *GhostClient { return &GhostClient{} }
