import * as Sentry from '@sentry/react'
import type { RootOptions } from 'react-dom/client'

// Browser error reporting. src/main.tsx imports this file only when the Instance set
// SPHERICON_SENTRY_DSN (src/instance.ts), so an Instance without one loads none of it.
//
// React 19 reports render errors through the root's own handlers, not through window.onerror:
// an error a boundary catches (TanStack Router wraps every route in one) goes to onCaughtError,
// one nothing catches to onUncaughtError. A top-level ErrorBoundary would miss the first kind
// (the router's boundaries sit below it), so the handlers go on the root. Errors outside React
// (window.onerror, unhandled promise rejections) come from the SDK's own global handlers.
export function startReporting(dsn: string): RootOptions {
  Sentry.init({ dsn })
  const report = Sentry.reactErrorHandler()
  // React types the component stack as optional, Sentry's as nullable.
  const handler = (error: unknown, info: { componentStack?: string | undefined }) => {
    report(error, { componentStack: info.componentStack ?? null })
  }
  return { onCaughtError: handler, onUncaughtError: handler, onRecoverableError: handler }
}
