import type { QueryClient } from '@tanstack/react-query'
import { createRouter } from '@tanstack/react-router'

import { AppErrorPage } from './components/AppErrorPage.tsx'
import { NotFoundPage } from './components/NotFoundPage.tsx'
import { routeTree } from './routeTree.gen.ts'

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultNotFoundComponent: NotFoundPage,
    defaultErrorComponent: AppErrorPage,
    defaultPreload: 'intent',
    defaultPreloadStaleTime: 0,
    // Cross-fade between pages (the browser View Transitions API, wrapped around each navigation).
    // Off under test: a navigation that starts while the previous cross-fade is still animating
    // skips it, the router drops the transition's `ready` promise, and vitest fails the run on
    // the unhandled AbortError. Drop this when TanStack/router#8776 is fixed.
    defaultViewTransition: import.meta.env.MODE !== 'test',
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
