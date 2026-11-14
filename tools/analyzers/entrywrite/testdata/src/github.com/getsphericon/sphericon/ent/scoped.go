package ent

import "context"

type Scoped struct{}

func (*Scoped) Tag() *TagScoped                               { return &TagScoped{} }
func (*Scoped) Contact() *ContactScoped                       { return &ContactScoped{} }
func (*Scoped) Segment() *SegmentScoped                       { return &SegmentScoped{} }
func (*Scoped) Automation() *AutomationScoped                 { return &AutomationScoped{} }
func (*Scoped) WorkspaceID() int64                            { return 0 }
func (*Scoped) Workspace(context.Context) (*Workspace, error) { return nil, nil }

const (
	TypeOf_Scoped             = TypeOther
	EntityOf_Scoped           = "Workspace"
	RoleOf_Scoped_Tag         = RoleNone
	RoleOf_Scoped_Contact     = RoleNone
	RoleOf_Scoped_Segment     = RoleNone
	RoleOf_Scoped_Automation  = RoleNone
	RoleOf_Scoped_WorkspaceID = RoleNone
	RoleOf_Scoped_Workspace   = RoleRead
)

type TagScoped struct{}

func (*TagScoped) Create() *TagCreate               { return &TagCreate{} }
func (*TagScoped) Update() *TagUpdate               { return &TagUpdate{} }
func (*TagScoped) CreateBulk() *TagCreate           { return &TagCreate{} }
func (*TagScoped) Query() *TagQuery                 { return &TagQuery{} }
func (*TagScoped) Get(context.Context, int64) error { return nil }
func (*TagScoped) Delete() *TagScopedDelete         { return &TagScopedDelete{} }

const (
	TypeOf_TagScoped            = TypeWrapper
	EntityOf_TagScoped          = "Tag"
	RoleOf_TagScoped_Create     = RoleStart
	RoleOf_TagScoped_Update     = RoleStart
	RoleOf_TagScoped_CreateBulk = RoleStart
	RoleOf_TagScoped_Query      = RoleRead
	RoleOf_TagScoped_Get        = RoleRead
	RoleOf_TagScoped_Delete     = RoleStart
)

type TagScopedDelete struct{}

func (d *TagScopedDelete) Where(...int64) *TagScopedDelete { return d }
func (*TagScopedDelete) Exec(context.Context) (int, error) { return 0, nil }

const (
	TypeOf_TagScopedDelete       = TypeScopedDelete
	EntityOf_TagScopedDelete     = "Tag"
	RoleOf_TagScopedDelete_Where = RoleStep
	RoleOf_TagScopedDelete_Exec  = RoleStep
)

type ContactScoped struct{}

func (*ContactScoped) UpdateOneID(int64) *ContactScopedUpdateOne { return &ContactScopedUpdateOne{} }

const (
	TypeOf_ContactScoped             = TypeWrapper
	EntityOf_ContactScoped           = "Contact"
	RoleOf_ContactScoped_UpdateOneID = RoleStart
)

type ContactScopedUpdateOne struct{}

func (u *ContactScopedUpdateOne) AddTagIDs(...int64) *ContactScopedUpdateOne { return u }
func (*ContactScopedUpdateOne) Exec(context.Context) error                   { return nil }

const (
	TypeOf_ContactScopedUpdateOne           = TypeScopedUpdate
	EntityOf_ContactScopedUpdateOne         = "Contact"
	RoleOf_ContactScopedUpdateOne_AddTagIDs = RoleStep
	RoleOf_ContactScopedUpdateOne_Exec      = RoleStep
)

type SegmentScoped struct{}

func (*SegmentScoped) Archive(context.Context, int64) error { return nil }
func (*SegmentScoped) Purge() *SegmentScopedDelete          { return &SegmentScopedDelete{} }

const (
	TypeOf_SegmentScoped         = TypeWrapper
	EntityOf_SegmentScoped       = "Segment"
	RoleOf_SegmentScoped_Archive = RoleStart
	RoleOf_SegmentScoped_Purge   = RoleStart
)

type SegmentScopedDelete struct{}

func (*SegmentScopedDelete) Exec(context.Context) (int, error) { return 0, nil }

const (
	TypeOf_SegmentScopedDelete      = TypeScopedDelete
	EntityOf_SegmentScopedDelete    = "Segment"
	RoleOf_SegmentScopedDelete_Exec = RoleStep
)

type AutomationScoped struct{}

func (*AutomationScoped) Pointers() *AutomationPointers { return &AutomationPointers{} }

const (
	TypeOf_AutomationScoped          = TypeWrapper
	EntityOf_AutomationScoped        = "Automation"
	RoleOf_AutomationScoped_Pointers = RoleStart
)

type AutomationPointers struct{}

func (*AutomationPointers) CompareAndSwap(context.Context, int64, int64) error { return nil }

const (
	TypeOf_AutomationPointers                = TypePointers
	EntityOf_AutomationPointers              = "Automation"
	RoleOf_AutomationPointers_CompareAndSwap = RoleStart
)

// VersionedStore is what the owner of a Versioned definition binds its pointers to: a move on it
// carries on the write the owner started with Pointers.
type VersionedStore[O, V any] interface {
	CompareAndSwap(ctx context.Context, id, versionID int64) error
}

const (
	TypeOf_VersionedStore                = TypeOther
	EntityOf_VersionedStore              = ""
	RoleOf_VersionedStore_CompareAndSwap = RoleStep
)
