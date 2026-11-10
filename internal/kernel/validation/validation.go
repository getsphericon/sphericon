// Package validation is how a domain module refuses an input (ADR 0107). Every exported write of a
// module normalises and checks all of its input itself, whichever driver called it: HTTP and MCP
// meet the contract's bounds in ogen first and in the module again, while the CLI, the jobs, the
// use cases and the other modules meet them only in the module. The bounds come from the contract
// through the generated package bounds, never from a Go copy.
//
// A refusal has one form, made by this package's constructors: Refuse, RefuseField, the Conflict
// ones and the other kinds for a sentinel, Args and On to fill its message or name its field, and
// Fields for several fields at once. The rules every owner shares live here too: Name for the name
// of a named row, UserName for a User's, Email and AccountEmail for an address in its Email case,
// Password for a password. The server's error handler answers each refusal by its kind on every
// surface; a driver returns it as it is and checks no rule of a row itself (review rule 5).
package validation
