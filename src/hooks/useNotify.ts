import type { NotificationData } from '@mantine/notifications'
import { createContext, useContext } from 'react'

type Notify = (data: NotificationData) => string

// The `show` of the toast store a NotificationsProvider owns. Toasts go to that store, never to
// Mantine's module-global default one, so every mounted app (and every test render) has toasts
// of its own.
export const NotifyContext = createContext<Notify | null>(null)

// useNotify returns the function that shows a toast in the nearest NotificationsProvider.
export function useNotify(): Notify {
  const notify = useContext(NotifyContext)
  if (!notify) {
    throw new Error('useNotify needs a NotificationsProvider above it')
  }
  return notify
}
