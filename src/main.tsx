import { QueryClient } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import { App } from './App.tsx'
import { client } from './generated/site/client.gen.ts'
import { client as staffClient } from './generated/staff/client.gen.ts'
import { createAppRouter } from './router.ts'

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

const container = document.querySelector('#root')
if (!container) {
  throw new Error('Root element not found')
}

createRoot(container).render(
  <StrictMode>
    <App queryClient={queryClient} router={router} />
  </StrictMode>,
)
