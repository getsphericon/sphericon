import { createNotificationsStore, Notifications, showNotification } from '@mantine/notifications'
import type { NotificationData } from '@mantine/notifications'
import { useState } from 'react'
import type { ReactNode } from 'react'

import { NotifyContext } from '../hooks/useNotify.ts'

// Owns one toast store and renders its toasts. The app mounts one; a test render mounts its own,
// so a toast left by one test can never cover the next test's buttons.
export function NotificationsProvider({ children }: { children: ReactNode }) {
  const [{ store, notify }] = useState(() => {
    const created = createNotificationsStore()
    return {
      store: created,
      notify: (data: NotificationData) => showNotification(data, created),
    }
  })

  return (
    <NotifyContext value={notify}>
      <Notifications store={store} position="top-right" layout="stacked" />
      {children}
    </NotifyContext>
  )
}
