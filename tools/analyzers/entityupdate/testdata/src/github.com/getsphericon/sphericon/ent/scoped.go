package ent

type TagScoped struct{}

func (*TagScoped) Update() *TagUpdateOne           { return &TagUpdateOne{} }
func (*TagScoped) UpdateOneID(int64) *TagUpdateOne { return &TagUpdateOne{} }

const (
	TypeOf_TagScoped             = TypeWrapper
	EntityOf_TagScoped           = "Tag"
	RoleOf_TagScoped_Update      = RoleStart
	RoleOf_TagScoped_UpdateOneID = RoleStart
)
