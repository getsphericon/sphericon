// Package paths names the URL prefixes more than one package must agree on: the
// composition root mounts a surface under its prefix, the surface's own server strips
// it, and the cookie and rate-limit rules of that surface key on it.
package paths

// Site is the prefix of the app's API, /site.
const Site = "/site"

// API is the prefix of the external API, /api.
const API = "/api"

// Collect is the prefix of the tracker's ingest API, /collect.
const Collect = "/collect"

// Staff member is the prefix of the platform Staff member API (ADR 0026).
const Staff = "/staff"

// The operational endpoints, never authenticated nor limited: the liveness and readiness probes
// and the metrics.
const (
	Healthz = "/healthz"
	Readyz  = "/readyz"
	Metrics = "/metrics"
)

// StaffJobs is where the job queue's UI (River UI) is served to a signed-in Staff member: under
// Staff, so the Staff member cookie reaches it.
const StaffJobs = Staff + "/jobs"

// Service is the prefix of the service endpoints of the Staff surface, served only
// by the internal listener (ADR 0026).
const Service = "/service"

// TLSAsk is the path of the edge's on-demand TLS question (ADR 0038), served only by the
// internal listener.
const TLSAsk = "/tls/ask"

// The link surface: the signed links a message carries, each its prefix and then its token. The
// message tokens build them, the links' handler routes them, the tracking host serves only them
// and the access log keeps their token out.
const (
	// Links is the prefix of the email links, mounted as one handler.
	Links = "/e/"
	// LinkOpen is the open pixel of an email.
	LinkOpen = Links + "o/"
	// LinkClick is a tracked link of an email.
	LinkClick = Links + "c/"
	// LinkUnsubscribe is the unsubscribe link of an email (and its one-click POST).
	LinkUnsubscribe = Links + "u/"
	// LinkConfirm is the confirmation link of a double opt-in.
	LinkConfirm = Links + "confirm/"
	// ShortLink is the short tracked link of a Telegram message or a text (ADR 0083).
	ShortLink = "/c/"
)

// LinkPrefixes is every prefix of the link surface.
var LinkPrefixes = []string{LinkOpen, LinkClick, LinkUnsubscribe, LinkConfirm, ShortLink}

// Hooks is the prefix of an Integration's inbound hook, followed by its ingest key (ADR 0037).
const Hooks = "/hooks/"
