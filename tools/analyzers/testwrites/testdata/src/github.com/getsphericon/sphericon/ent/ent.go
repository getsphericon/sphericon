// Package ent stands in for the generated ent package.
package ent

type TagCreate struct{}

func (*TagCreate) SetName(string) *TagCreate { return nil }
func (*TagCreate) SaveX() int                { return 0 }

type TagCreateBulk struct{}

type TagClient struct{}

func (*TagClient) Create() *TagCreate { return nil }

func (*TagClient) CreateBulk(...*TagCreate) *TagCreateBulk { return nil }

type TagScoped struct{}

func (*TagScoped) Create() *TagCreate { return nil }

type Client struct{ Tag *TagClient }

type Scoped struct{}

func (*Scoped) Tag() *TagScoped { return nil }
