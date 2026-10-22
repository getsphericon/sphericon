// Package ent is a stub of the generated ent package: the builders of a state moment.
package ent

import (
	"context"
	"time"
)

type Predicate struct{}

type SendingDomainUpdateOne struct{}

func (x *SendingDomainUpdateOne) Where(...Predicate) *SendingDomainUpdateOne { return x }
func (x *SendingDomainUpdateOne) SetVerifiedAt(time.Time) *SendingDomainUpdateOne {
	return x
}
func (x *SendingDomainUpdateOne) SetVerificationLostAt(time.Time) *SendingDomainUpdateOne {
	return x
}
func (x *SendingDomainUpdateOne) SetLastCheckedAt(time.Time) *SendingDomainUpdateOne {
	return x
}
func (*SendingDomainUpdateOne) Exec(context.Context) error { return nil }

type SendingDomainScopedUpdateOne struct{}

func (x *SendingDomainScopedUpdateOne) Where(...Predicate) *SendingDomainScopedUpdateOne {
	return x
}
func (x *SendingDomainScopedUpdateOne) SetVerificationLostAt(time.Time) *SendingDomainScopedUpdateOne {
	return x
}
func (*SendingDomainScopedUpdateOne) Exec(context.Context) error { return nil }

type SendingDomainCreate struct{}

func (x *SendingDomainCreate) SetVerifiedAt(time.Time) *SendingDomainCreate { return x }

type WebhookEndpointUpdate struct{}

func (x *WebhookEndpointUpdate) Where(...Predicate) *WebhookEndpointUpdate { return x }
func (x *WebhookEndpointUpdate) SetDisabledAt(time.Time) *WebhookEndpointUpdate {
	return x
}
func (x *WebhookEndpointUpdate) ClearDisabledAt() *WebhookEndpointUpdate { return x }

type JoinDomainUpdateOne struct{}

func (x *JoinDomainUpdateOne) Where(...Predicate) *JoinDomainUpdateOne { return x }
func (x *JoinDomainUpdateOne) ClearVerifiedAt() *JoinDomainUpdateOne   { return x }

type UsageUpdateOne struct{}

func (x *UsageUpdateOne) Where(...Predicate) *UsageUpdateOne                { return x }
func (x *UsageUpdateOne) SetFinalizedAt(time.Time) *UsageUpdateOne          { return x }
func (x *UsageUpdateOne) SetNillableFinalizedAt(*time.Time) *UsageUpdateOne { return x }

type TagUpdateOne struct{}

func (x *TagUpdateOne) SetVerifiedAt(time.Time) *TagUpdateOne { return x }

type Client struct{}

func (*Client) SendingDomain() *SendingDomainUpdateOne { return &SendingDomainUpdateOne{} }
func (*Client) ScopedSendingDomain() *SendingDomainScopedUpdateOne {
	return &SendingDomainScopedUpdateOne{}
}
func (*Client) WebhookEndpoint() *WebhookEndpointUpdate { return &WebhookEndpointUpdate{} }
func (*Client) JoinDomain() *JoinDomainUpdateOne        { return &JoinDomainUpdateOne{} }
func (*Client) Usage() *UsageUpdateOne                  { return &UsageUpdateOne{} }
func (*Client) Tag() *TagUpdateOne                      { return &TagUpdateOne{} }
func (*Client) NewSendingDomain() *SendingDomainCreate  { return &SendingDomainCreate{} }
