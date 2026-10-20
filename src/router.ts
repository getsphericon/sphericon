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
    defaultViewTransition: true,
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
