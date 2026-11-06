// Package db opens the database and guards its schema: the connection pools, the goose migrations
// and the checks that a Major upgrade did not skip a release. It builds no application client:
// the wiring does, with the hooks other modules install (internal/app/entclient). Its one client
// is a bare one for the guard's read of the schema state row.
package db
