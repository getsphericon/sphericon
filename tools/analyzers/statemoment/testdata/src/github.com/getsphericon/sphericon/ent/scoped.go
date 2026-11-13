package ent

import "time"

type SendingDomainScopedUpdateOne struct{}

func (x *SendingDomainScopedUpdateOne) Where(...Predicate) *SendingDomainScopedUpdateOne { return x }
func (x *SendingDomainScopedUpdateOne) SetVerifiedAt(time.Time) *SendingDomainScopedUpdateOne {
	return x
}

const (
	TypeOf_SendingDomainScopedUpdateOne                  = TypeScopedUpdate
	EntityOf_SendingDomainScopedUpdateOne                = "SendingDomain"
	RoleOf_SendingDomainScopedUpdateOne_Where            = RoleWrite
	RoleOf_SendingDomainScopedUpdateOne_SetVerifiedAt    = RoleWrite
	SetsState_SendingDomainScopedUpdateOne_SetVerifiedAt = "VerifiedAt"
)
