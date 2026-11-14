// Package ent is a stand-in of the generated ent package: entities whose Update starts a write past
// the scope, the builders and the scoped wrapper whose Update does not, and the manifest of roles
// (roles.go, scoped.go), which ent/schema/roles_test.go holds to the rules of the real one.
package ent

import "context"

type Tag struct{}

func (*Tag) Update() *TagUpdateOne { return &TagUpdateOne{} }

// OAuthClient is an entity whose name ends in Client.
type OAuthClient struct{}

func (*OAuthClient) Update() *OAuthClientUpdateOne { return &OAuthClientUpdateOne{} }

type OAuthClientUpdateOne struct{}

func (*OAuthClientUpdateOne) Exec(context.Context) error { return nil }

type TagUpdateOne struct{}

func (*TagUpdateOne) Exec(context.Context) error { return nil }

type TagClient struct{}

func (*TagClient) Update() *TagUpdateOne { return &TagUpdateOne{} }

type TagUpsertOne struct{}

func (u *TagUpsertOne) Update(func()) *TagUpsertOne { return u }
