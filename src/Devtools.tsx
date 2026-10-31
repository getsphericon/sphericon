import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'

import type { AppRouter } from './router.ts'

// The query cache and the route tree, inspectable in a development build. App loads this only
// there, so a production bundle carries neither.
export function Devtools({ router }: { router: AppRouter }) {
  return (
    <>
      <ReactQueryDevtools buttonPosition="bottom-left" />
      <TanStackRouterDevtools router={router} position="bottom-right" />
    </>
  )
}
