import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Outlet } from '@tanstack/react-router'
import { ViewTransition } from 'react'

// The root is cross-cutting concerns only. Layout chrome lives in the per-area layouts
// (WorkspaceLayout, AccountLayout); auth pages render bare. Page transitions live here too:
// the router commits every navigation inside a React transition, so ViewTransition
// cross-fades the routed tree.
//
// The dashboard intentionally does NOT self-track into customer workspaces: that would
// pollute their event stream and break the install-status check. Dashboard product
// analytics, if added later, must use a separate collect key.
export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: () => (
    <ViewTransition>
      <Outlet />
    </ViewTransition>
  ),
})
