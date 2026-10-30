import { createFileRoute, redirect } from '@tanstack/react-router'

import { Route as consoleWorkspacesRoute } from './_enterprise/workspaces.index.tsx'

// The console's home. A console that serves the Enterprise sections opens on the Workspaces, as it
// did before the core sections existed (ADR 0063); the core home is for the console without them.
export const Route = createFileRoute('/console/_authed/')({
  beforeLoad: ({ context }) => {
    if (context.enterprise) {
      throw redirect({ to: consoleWorkspacesRoute.to })
    }
  },
})
