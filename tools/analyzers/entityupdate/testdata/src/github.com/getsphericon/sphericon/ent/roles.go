package ent

// The vocabulary of the manifest of roles.
const (
	RoleRead  = "read"
	RoleStart = "start"
	RoleWrite = "write"
	RoleNone  = "none"
)

const (
	TypeEntity  = "entity"
	TypeClient  = "client"
	TypeUpdate  = "update"
	TypeUpsert  = "upsert"
	TypeWrapper = "wrapper"
	TypeOther   = "other"
)

// The types ent generates and the writes their methods start.
const (
	TypeOf_Tag                    = TypeEntity
	EntityOf_Tag                  = "Tag"
	RoleOf_Tag_Update             = RoleStart
	TypeOf_OAuthClient            = TypeEntity
	EntityOf_OAuthClient          = "OAuthClient"
	RoleOf_OAuthClient_Update     = RoleStart
	TypeOf_OAuthClientUpdateOne   = TypeUpdate
	EntityOf_OAuthClientUpdateOne = "OAuthClient"
	TypeOf_TagUpdateOne           = TypeUpdate
	EntityOf_TagUpdateOne         = "Tag"
	TypeOf_TagClient              = TypeClient
	EntityOf_TagClient            = "Tag"
	RoleOf_TagClient_Update       = RoleStart
	TypeOf_TagUpsertOne           = TypeUpsert
	EntityOf_TagUpsertOne         = "Tag"
)
