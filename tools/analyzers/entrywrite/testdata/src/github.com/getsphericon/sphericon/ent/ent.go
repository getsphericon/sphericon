// Package ent is a stub of the generated ent package: the client, the scoped client and a builder.
package ent

import "context"

type TagCreate struct{}

func (c *TagCreate) SetName(string) *TagCreate { return c }

func (*TagCreate) Save(context.Context) error { return nil }

func (c *TagCreate) OnConflictColumns(...string) *TagCreate { return c }

func (c *TagCreate) OnConflict() *TagCreate { return c }

type TagUpdate struct{}

func (*TagUpdate) Exec(context.Context) error { return nil }

type TagQuery struct{}

func (*TagQuery) All(context.Context) ([]string, error) { return nil, nil }

type TagClient struct{}

func (*TagClient) Create() *TagCreate           { return &TagCreate{} }
func (*TagClient) UpdateOneID(int64) *TagUpdate { return &TagUpdate{} }
func (*TagClient) DeleteOneID(int64) *TagUpdate { return &TagUpdate{} }
func (*TagClient) Query() *TagQuery             { return &TagQuery{} }

type Client struct{ Tag *TagClient }

type TagScoped struct{}

func (*TagScoped) Create() *TagCreate               { return &TagCreate{} }
func (*TagScoped) Update() *TagUpdate               { return &TagUpdate{} }
func (*TagScoped) CreateBulk() *TagCreate           { return &TagCreate{} }
func (*TagScoped) Query() *TagQuery                 { return &TagQuery{} }
func (*TagScoped) Get(context.Context, int64) error { return nil }

type Tag struct{}

func (*Tag) QueryContacts() *TagQuery { return &TagQuery{} }

type Workspace struct{}

type Scoped struct{}

func (*Scoped) Tag() *TagScoped                               { return &TagScoped{} }
func (*Scoped) WorkspaceID() int64                            { return 0 }
func (*Scoped) Workspace(context.Context) (*Workspace, error) { return nil, nil }
