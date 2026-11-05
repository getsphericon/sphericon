// Package db opens the database and guards its schema: the connection pools, the goose migrations
// and the checks that a Major upgrade did not skip a release. It builds no ent client: the wiring
// does, with the hooks other modules install (internal/app/entclient).
package db
