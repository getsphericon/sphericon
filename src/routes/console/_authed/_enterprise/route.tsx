import { createFileRoute, redirect } from '@tanstack/react-router'

import { Route as consoleHomeRoute } from '../index.tsx'

// The Enterprise sections of the console (ADR 0063): reachable only when the server serves them,
// otherwise a link or a typed URL lands on the console home.
export const Route = createFileRoute('/console/_authed/_enterprise')({
  beforeLoad: ({ context }) => {
    if (!context.enterprise) {
      throw redirect({ to: consoleHomeRoute.to })
    }
  },
})
