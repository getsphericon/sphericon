import { MantineProvider } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { I18nextProvider } from 'react-i18next'

import { NotificationsProvider } from './components/NotificationsProvider.tsx'
import { editorCssVariablesResolver } from './editorCssVariables.ts'
import { i18n } from './i18n.ts'
import type { AppRouter } from './router.ts'
import { theme } from './theme.ts'

// The app's providers around the router. It lives apart from main.tsx so an edit here hot-reloads:
// main.tsx is the entry and owns the long-lived queryClient and router, which a refresh must keep.
export function App({ queryClient, router }: { queryClient: QueryClient; router: AppRouter }) {
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
