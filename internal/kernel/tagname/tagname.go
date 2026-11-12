// Package tagname says when two Tag names are one Tag: around its trimmed text and without regard
// to case, so "VIP" and "vip" are one Tag (GLOSSARY). The Tag keeps the name as first written; every
// comparison of a name goes through Key: the stored key (tags.name_key, unique in a Workspace), a
// lookup by name, and a definition (a Condition, a step, an Action) naming a Tag. Postgres never
// computes it, so the database and the code cannot fold a name differently.
package tagname

import "strings"

// Key is the form a Tag name is matched in.
func Key(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Same reports whether two names are one Tag's.
func Same(a, b string) bool { return Key(a) == Key(b) }
