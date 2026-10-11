// Package paths names the URL prefixes more than one package must agree on: the
// composition root mounts a surface under its prefix, the surface's own server strips
// it, and the cookie and rate-limit rules of that surface key on it.
package paths

// Operator is the prefix of the platform Operator API (ADR 0026).
const Operator = "/operator"
