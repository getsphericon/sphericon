import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { I18nextProvider } from 'react-i18next'

import { NotificationsProvider } from './components/NotificationsProvider.tsx'
import { editorCssVariablesResolver } from './editorCssVariables.ts'
import { client } from './generated/site/client.gen.ts'
import { client as staffClient } from './generated/staff/client.gen.ts'
import { i18n } from './i18n.ts'
import { createAppRouter } from './router.ts'
import { theme } from './theme.ts'

client.setConfig({ baseUrl: '/site' })
staffClient.setConfig({ baseUrl: '/staff' })

import '@mantine/core/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/notifications/styles.css'
import 'mantine-datatable/styles.css'
import '@react-email/editor/themes/default.css'
// Structural layout only; the Mantine compat package handles the visuals.
import 'react-querybuilder/dist/query-builder-layout.css'

// The workflow builder SDK stylesheet is intentionally NOT imported here: it
// ships unlayered global resets (body{overflow:hidden}, *{font-family}) and
// rewrites global .mantine-* classes, which leak across the whole app. It is
// injected only while the builder is mounted — see AutomationBuilder.tsx.

const queryClient = new QueryClient()
const router = createAppRouter(queryClient)

function Root() {
  return (
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <MantineProvider
          theme={theme}
          defaultColorScheme="auto"
          cssVariablesResolver={editorCssVariablesResolver}
        >
          <ModalsProvider>
            <NotificationsProvider>
              <RouterProvider router={router} />
            </NotificationsProvider>
          </ModalsProvider>
        </MantineProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

const container = document.querySelector('#root')
if (!container) {
  throw new Error('Root element not found')
}

createRoot(container).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
