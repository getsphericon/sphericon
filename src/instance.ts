// Whether the instance requires a proven email address before an account can sign in
// (ADR 0030). The backend substitutes window.__REQUIRE_EMAIL_VERIFICATION__ into
// index.html at serve time (see internal/server/spa/spa.go); Vite does the same in dev.
// Anything but an explicit `false` keeps the gate on, as in the backend's config.
export function requiresEmailVerification(): boolean {
  return Reflect.get(window, '__REQUIRE_EMAIL_VERIFICATION__') !== false
}

// The DSN browser errors are reported to, from SPHERICON_SENTRY_DSN (substituted into
// index.html as window.__SENTRY_DSN__, see internal/server/spa/spa.go). Empty, the default and
// whatever is not a string, means reporting is off.
export function sentryDsn(): string {
  const dsn: unknown = Reflect.get(window, '__SENTRY_DSN__')
  return typeof dsn === 'string' ? dsn.trim() : ''
}
