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
	TypeCreate       = "create"
	TypeUpdate       = "update"
	TypeUpsert       = "upsert"
	TypeScopedUpdate = "scoped update"
	TypeOther        = "other"
)

// The types ent generates and the state fields their methods write.
const (
	TypeOf_Client      = TypeOther
	EntityOf_Client    = ""
	TypeOf_Predicate   = TypeOther
	EntityOf_Predicate = ""

	TypeOf_SendingDomain                               = TypeEntity
	EntityOf_SendingDomain                             = "SendingDomain"
	TypeOf_SendingDomainUpdateOne                      = TypeUpdate
	EntityOf_SendingDomainUpdateOne                    = "SendingDomain"
	SetsState_SendingDomainUpdateOne_SetVerifiedAt     = "VerifiedAt"
	ClearsState_SendingDomainUpdateOne_ClearVerifiedAt = "VerifiedAt"
	TypeOf_SendingDomainUpsert                         = TypeUpsert
	EntityOf_SendingDomainUpsert                       = "SendingDomain"
	SetsState_SendingDomainUpsert_UpdateVerifiedAt     = "VerifiedAt"
	TypeOf_SendingDomainCreate                         = TypeCreate
	EntityOf_SendingDomainCreate                       = "SendingDomain"

	TypeOf_Invitation                                = TypeEntity
	EntityOf_Invitation                              = "Invitation"
	TypeOf_InvitationUpdate                          = TypeUpdate
	EntityOf_InvitationUpdate                        = "Invitation"
	SetsState_InvitationUpdate_SetAcceptedAt         = "AcceptedAt"
	SetsState_InvitationUpdate_SetNillableAcceptedAt = "AcceptedAt"
	ClearsState_InvitationUpdate_ClearAcceptedAt     = "AcceptedAt"

	TypeOf_Tag            = TypeEntity
	EntityOf_Tag          = "Tag"
	TypeOf_TagUpdateOne   = TypeUpdate
	EntityOf_TagUpdateOne = "Tag"
)
