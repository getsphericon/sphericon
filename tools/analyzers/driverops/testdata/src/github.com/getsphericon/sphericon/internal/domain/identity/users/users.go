package users

import "context"

type User struct{ ID int64 }

type Module struct{}

func (*Module) Get(ctx context.Context, id int64) (*User, error)     { return nil, nil }
func (*Module) Rename(ctx context.Context, id int64, n string) error { return nil }

// View maps without a context: mapping is no operation.
func View(u *User) string { return "" }

func Lookup(ctx context.Context, email string) (*User, error) { return nil, nil }
