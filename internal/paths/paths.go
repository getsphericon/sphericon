// Package paths names the URL prefixes more than one package must agree on: the
// composition root mounts a surface under its prefix, the surface's own server strips
// it, and the cookie and rate-limit rules of that surface key on it.
package paths

// Staff member is the prefix of the platform Staff member API (ADR 0026).
const Staff = "/staff"

// StaffJobs is where the job queue's UI (River UI) is served to a signed-in Staff member: under
// Staff, so the Staff member cookie reaches it.
const StaffJobs = Staff + "/jobs"

// Service is the prefix of the service endpoints of the Staff surface, served only
// by the internal listener (ADR 0026).
const Service = "/service"

// TLSAsk is the path of the edge's on-demand TLS question (ADR 0038), served only by the
// internal listener.
const TLSAsk = "/tls/ask"
