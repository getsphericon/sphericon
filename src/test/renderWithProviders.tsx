import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { render } from 'vitest-browser-react'

import { NotificationsProvider } from '../components/NotificationsProvider.tsx'
import { trackQueryClient } from './queryClients.ts'

// Mirrors the provider stack from src/main.tsx so components render the same way
// they do in the app. A fresh QueryClient per render keeps tests isolated;
// retries are off so failed requests surface immediately instead of after delays.
// Mantine's `env="test"` turns its transitions and portals off: without it every menu, modal
// and toast animates, each Playwright action waits for the element to stop moving, and the
// suite takes about 40% longer and times out under load.
//
// Components that depend on the router (useNavigate/useParams) need the router
// wrapper from renderWithRouter.tsx instead.
function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

export function renderWithProviders(ui: ReactNode) {
  const queryClient = makeQueryClient()
  trackQueryClient(queryClient)

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MantineProvider env="test">
          <ModalsProvider>
            <NotificationsProvider>{children}</NotificationsProvider>
          </ModalsProvider>
        </MantineProvider>
      </QueryClientProvider>
    )
  }

  return render(ui, { wrapper: Wrapper })
}
