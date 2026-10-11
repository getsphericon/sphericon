import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { client as operatorClient } from './generated/operator/client.gen.ts'
import { client } from './generated/site/client.gen.ts'
import { i18n } from './i18n.ts'
import { router } from './router.tsx'

client.setConfig({ baseUrl: '/site' })
operatorClient.setConfig({ baseUrl: '/operator' })

import '@mantine/core/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/notifications/styles.css'
import 'mantine-datatable/styles.css'
// Structural layout only; the Mantine compat package handles the visuals.
import 'react-querybuilder/dist/query-builder-layout.css'

// The workflow builder SDK stylesheet is intentionally NOT imported here: it
// ships unlayered global resets (body{overflow:hidden}, *{font-family}) and
// rewrites global .mantine-* classes, which leak across the whole app. It is
// injected only while the builder is mounted — see AutomationBuilder.tsx.

function Root() {
  const [queryClient] = useState(() => new QueryClient())

  return (
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <MantineProvider defaultColorScheme="auto">
          <ModalsProvider>
            <Notifications position="top-right" />
            <RouterProvider router={router} />
          </ModalsProvider>
        </MantineProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

const container = document.getElementById('root')
if (!container) {
  throw new Error('Root element not found')
}

createRoot(container).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
