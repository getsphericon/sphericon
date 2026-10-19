import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { render } from 'vitest-browser-react'

import { trackQueryClient } from './queryClients.ts'

// Mirrors the provider stack from src/main.tsx so components render the same way
// they do in the app. A fresh QueryClient per render keeps tests isolated;
// retries are off so failed requests surface immediately instead of after delays.
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
        <MantineProvider>
          <ModalsProvider>
            <Notifications position="top-right" layout="stacked" />
            {children}
          </ModalsProvider>
        </MantineProvider>
      </QueryClientProvider>
    )
  }

  return render(ui, { wrapper: Wrapper })
}
