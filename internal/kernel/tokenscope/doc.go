// Package tokenscope is the scope vocabulary of the credentials a User gives (ADR 0061): the
// scopes an OAuth client is granted by default, those that need the User's explicit opt-in, and
// the Personal scopes a Connection or a Personal token carries. cmd/scopegen generates it from
// the TypeSpec contract, the vocabulary's single place, so a core package reads it without
// importing a surface's generated code.
package tokenscope
