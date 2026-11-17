// Package ent stands in for the generated ent package, with the part of its manifest of roles
// (ent/template/roles.tmpl) that says which methods start a create.
package ent

// The vocabulary of the manifest of roles.
const (
	RoleRead         = "read"
	RoleStart        = "start"
	RoleStep         = "step"
	TypeCreate       = "create"
	TypeScopedCreate = "scoped_create"
	TypeQuery        = "query"
	TypeOther        = "other"
)

// What each type is and the role of each method.
const (
	TypeOf_TagCreate               = TypeCreate
	TypeOf_TagCreateBulk           = TypeCreate
	TypeOf_TagScopedCreate         = TypeScopedCreate
	TypeOf_TagQuery                = TypeQuery
	RoleOf_TagCreate_SetName       = RoleStep
	RoleOf_TagCreate_SaveX         = RoleStep
	RoleOf_TagScopedCreate_SetName = RoleStep
	RoleOf_TagScopedCreate_SaveX   = RoleStep
	RoleOf_TagClient_Create        = RoleStart
	RoleOf_TagClient_CreateBulk    = RoleStart
	RoleOf_TagClient_MapCreateBulk = RoleStart
	RoleOf_TagClient_Query         = RoleRead
	RoleOf_TagScoped_Create        = RoleStart
)

type TagCreate struct{}

func (*TagCreate) SetName(string) *TagCreate { return nil }
func (*TagCreate) SaveX() int                { return 0 }

type TagCreateBulk struct{}

type TagScopedCreate struct{}

func (*TagScopedCreate) SetName(string) *TagScopedCreate { return nil }
func (*TagScopedCreate) SaveX() int                      { return 0 }

type TagQuery struct{}

type TagClient struct{}

func (*TagClient) Create() *TagCreate { return nil }

func (*TagClient) CreateBulk(...*TagCreate) *TagCreateBulk { return nil }

func (*TagClient) MapCreateBulk(any, func(*TagCreate, int)) *TagCreateBulk { return nil }

func (*TagClient) Query() *TagQuery { return nil }

type TagScoped struct{}

func (*TagScoped) Create() *TagScopedCreate { return nil }

type Client struct{ Tag *TagClient }

type Scoped struct{}

func (*Scoped) Tag() *TagScoped { return nil }
