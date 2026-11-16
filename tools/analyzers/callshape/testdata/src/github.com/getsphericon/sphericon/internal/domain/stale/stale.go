// Package stale keeps the call contract but is still on the exemption list.
package stale // want `rule 5: internal/domain/stale is on the exemption list but keeps the call contract`

// Normalize is a pure helper.
func Normalize(s string) string { return s }
