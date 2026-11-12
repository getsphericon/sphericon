// Package fieldname names the contract's request fields (fieldnames_gen.go, written by
// cmd/scopegen from every OpenAPI document): a refusal of a field is reported under its json name
// (validation.RefuseField), and the app shows it under the field of that name. A module names the
// field by its constant here, so a name the contract does not have fails to compile.
package fieldname
