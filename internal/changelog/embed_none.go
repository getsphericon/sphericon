//go:build !embed_changelog

package changelog

// Embedded is the changelog of this build; empty in a build without the embed_changelog tag.
func Embedded() string { return "" }
