import { TanStackDevtools } from '@tanstack/react-devtools'
import { ReactQueryDevtoolsPanel } from '@tanstack/react-query-devtools'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'

import type { AppRouter } from './router.ts'

// The query cache and the route tree in one TanStack devtools panel, in a development build. App
// loads this only there, so a production bundle carries none of it.
export function Devtools({ router }: { router: AppRouter }) {
  return (
    <TanStackDevtools
      // The default bottom-right corner covers the Save button of a form.
      config={{ position: 'middle-right' }}
      plugins={[
        { name: 'TanStack Query', render: <ReactQueryDevtoolsPanel /> },
        { name: 'TanStack Router', render: <TanStackRouterDevtoolsPanel router={router} /> },
      ]}
    />
  )
}
