package ent

// The vocabulary of the manifest of roles.
const (
	RoleRead  = "read"
	RoleStart = "start"
	RoleWrite = "write"
	RoleNone  = "none"
)

const (
	TypeEntity       = "entity"
	TypeClient       = "client"
	TypeQuery        = "query"
	TypeCreate       = "create"
	TypeUpdate       = "update"
	TypeDelete       = "delete"
	TypeUpsert       = "upsert"
	TypeWrapper      = "wrapper"
	TypeScopedCreate = "scoped create"
	TypeScopedUpdate = "scoped update"
	TypeScopedDelete = "scoped delete"
	TypeScopedUpsert = "scoped upsert"
	TypePointers     = "pointers"
	TypeOther        = "other"
)

// The types ent generates and the reads and writes their methods start.
const (
	TypeOf_Client   = TypeOther
	EntityOf_Client = ""

	TypeOf_Workspace   = TypeEntity
	EntityOf_Workspace = "Workspace"

	TypeOf_Tag                         = TypeEntity
	EntityOf_Tag                       = "Tag"
	RoleOf_Tag_QueryContacts           = RoleRead
	TypeOf_TagCreate                   = TypeCreate
	EntityOf_TagCreate                 = "Tag"
	RoleOf_TagCreate_OnConflict        = RoleStart
	RoleOf_TagCreate_OnConflictColumns = RoleStart
	TypeOf_TagUpdate                   = TypeUpdate
	EntityOf_TagUpdate                 = "Tag"
	TypeOf_TagDeleteOne                = TypeDelete
	EntityOf_TagDeleteOne              = "Tag"
	TypeOf_TagQuery                    = TypeQuery
	EntityOf_TagQuery                  = "Tag"
	TypeOf_TagClient                   = TypeClient
	EntityOf_TagClient                 = "Tag"
	RoleOf_TagClient_Create            = RoleStart
	RoleOf_TagClient_UpdateOneID       = RoleStart
	RoleOf_TagClient_DeleteOneID       = RoleStart
	RoleOf_TagClient_Query             = RoleRead

	TypeOf_Contact      = TypeEntity
	EntityOf_Contact    = "Contact"
	TypeOf_Segment      = TypeEntity
	EntityOf_Segment    = "Segment"
	TypeOf_Automation   = TypeEntity
	EntityOf_Automation = "Automation"

	TypeOf_Ghost              = TypeEntity
	EntityOf_Ghost            = "Ghost"
	TypeOf_GhostCreate        = TypeCreate
	EntityOf_GhostCreate      = "Ghost"
	TypeOf_GhostClient        = TypeClient
	EntityOf_GhostClient      = "Ghost"
	RoleOf_GhostClient_Create = RoleStart
)
