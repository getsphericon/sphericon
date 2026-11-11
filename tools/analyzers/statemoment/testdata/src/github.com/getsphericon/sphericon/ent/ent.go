// Package ent is a stub of the generated ent package: the forms ent/entc.go declares and the builders
// that write them.
package ent

import (
	"context"
	"time"
)

const (
	StateFormSendingDomainVerifiedAt = "machine"
	StateFormInvitationAcceptedAt    = "latch"
)

type Predicate struct{}

type SendingDomainUpdateOne struct{}

func (x *SendingDomainUpdateOne) Where(...Predicate) *SendingDomainUpdateOne         { return x }
func (x *SendingDomainUpdateOne) SetVerifiedAt(time.Time) *SendingDomainUpdateOne    { return x }
func (x *SendingDomainUpdateOne) ClearVerifiedAt() *SendingDomainUpdateOne           { return x }
func (x *SendingDomainUpdateOne) SetLastCheckedAt(time.Time) *SendingDomainUpdateOne { return x }
func (*SendingDomainUpdateOne) Save(context.Context) (int, error)                    { return 0, nil }

type SendingDomainScopedUpdateOne struct{}

func (x *SendingDomainScopedUpdateOne) Where(...Predicate) *SendingDomainScopedUpdateOne { return x }
func (x *SendingDomainScopedUpdateOne) SetVerifiedAt(time.Time) *SendingDomainScopedUpdateOne {
	return x
}

type SendingDomainUpsert struct{}

func (x *SendingDomainUpsert) UpdateVerifiedAt() *SendingDomainUpsert { return x }

type SendingDomainCreate struct{}

func (x *SendingDomainCreate) SetVerifiedAt(time.Time) *SendingDomainCreate { return x }

type InvitationUpdate struct{}

func (x *InvitationUpdate) Where(...Predicate) *InvitationUpdate               { return x }
func (x *InvitationUpdate) SetAcceptedAt(time.Time) *InvitationUpdate          { return x }
func (x *InvitationUpdate) ClearAcceptedAt() *InvitationUpdate                 { return x }
func (x *InvitationUpdate) SetNillableAcceptedAt(*time.Time) *InvitationUpdate { return x }
func (*InvitationUpdate) Save(context.Context) (int, error)                    { return 0, nil }

type TagUpdateOne struct{}

func (x *TagUpdateOne) SetVerifiedAt(time.Time) *TagUpdateOne { return x }

type Client struct{}

func (*Client) SendingDomain() *SendingDomainUpdateOne { return &SendingDomainUpdateOne{} }
func (*Client) ScopedSendingDomain() *SendingDomainScopedUpdateOne {
	return &SendingDomainScopedUpdateOne{}
}
func (*Client) SendingDomainUpsert() *SendingDomainUpsert { return &SendingDomainUpsert{} }
func (*Client) NewSendingDomain() *SendingDomainCreate    { return &SendingDomainCreate{} }
func (*Client) Invitation() *InvitationUpdate             { return &InvitationUpdate{} }
func (*Client) Tag() *TagUpdateOne                        { return &TagUpdateOne{} }
