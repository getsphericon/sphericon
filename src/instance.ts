// Whether the instance requires a proven email address before an account can sign in
// (ADR 0030). The backend substitutes window.__REQUIRE_EMAIL_VERIFICATION__ into
// index.html at serve time (see internal/server/spa.go); Vite does the same in dev.
// Anything but an explicit `false` keeps the gate on, as in the backend's config.
export function requiresEmailVerification(): boolean {
  return Reflect.get(window, '__REQUIRE_EMAIL_VERIFICATION__') !== false
}
