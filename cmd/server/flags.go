package main

// The names of the flags and arguments the commands read back, each declared and looked up by
// its constant so a misspelt lookup fails to compile instead of reading a zero value.
const (
	flagBefore    = "before"
	flagCheck     = "check"
	flagConsumer  = "consumer"
	flagDryRun    = "dry-run"
	flagEmail     = "email"
	flagEventID   = "event-id"
	flagExpiresIn = "expires-in"
	flagFrom      = "from"
	flagHeader    = "header"
	flagID        = "id"
	flagInstance  = "instance"
	flagJSON      = "json"
	flagLimit     = "limit"
	flagName      = "name"
	flagNow       = "now"
	flagParam     = "param"
	flagPayload   = "payload"
	flagReason    = "reason"
	flagRestart   = "restart"
	flagScopes    = "scopes"
	flagService   = "service"
	flagSince     = "since"
	flagStatus    = "status"
	flagURL       = "url"
	flagVersion   = "version"
	flagWorkspace = "workspace"
	flagYes       = "yes"
)
