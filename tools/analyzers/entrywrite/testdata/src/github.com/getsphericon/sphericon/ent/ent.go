// Package ent is a stand-in of the generated ent package: the types ent generates (this file), the
// ones the templates emit (scoped.go) and the manifest of roles that describes them (roles.go), which
// ent/schema/roles_test.go holds to the rules of the real one.
package ent

import "context"

type Tag struct{}

func (*Tag) QueryContacts() *TagQuery { return &TagQuery{} }

type TagCreate struct{}

func (c *TagCreate) SetName(string) *TagCreate { return c }

func (*TagCreate) Save(context.Context) error { return nil }

func (c *TagCreate) OnConflictColumns(...string) *TagCreate { return c }

func (c *TagCreate) OnConflict() *TagCreate { return c }

type TagUpdate struct{}

func (*TagUpdate) Exec(context.Context) error { return nil }

type TagDeleteOne struct{}

func (*TagDeleteOne) Exec(context.Context) error { return nil }

type TagQuery struct{}

func (*TagQuery) All(context.Context) ([]string, error) { return nil, nil }

type TagClient struct{}

func (*TagClient) Create() *TagCreate              { return &TagCreate{} }
func (*TagClient) UpdateOneID(int64) *TagUpdate    { return &TagUpdate{} }
func (*TagClient) DeleteOneID(int64) *TagDeleteOne { return &TagDeleteOne{} }
func (*TagClient) Query() *TagQuery                { return &TagQuery{} }

type Client struct{ Tag *TagClient }

type Workspace struct{}

// The entities the ownership map is tested on: a second one with its own owner (Contact), an
// archivable one (Segment), a Versioned definition (Automation) and one the map does not name (Ghost).

type Contact struct{}

type Segment struct{}

type Automation struct{}

type Ghost struct{}

type GhostCreate struct{}

func (*GhostCreate) Save(context.Context) error { return nil }

type GhostClient struct{}

func (*GhostClient) Create() *GhostCreate { return &GhostCreate{} }

func (*Client) Ghost() *GhostClient { return &GhostClient{} }
