import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { siteAuthLogoutMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as loginRoute } from '../routes/login.tsx'
import { useNotify } from './useNotify.ts'

// useLogout signs the User out on this browser: the logout operation clears the session cookie,
// then the cached queries are dropped and the app routes to the login. A failure is toasted.
export function useLogout() {
  const { t } = useTranslation()
  const notify = useNotify()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  return useMutation({
    ...siteAuthLogoutMutation(),
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: loginRoute.to })
    },
    onError: () => {
      notify({
        color: 'red',
        title: t(($) => $.userMenu.logoutErrorTitle),
        message: t(($) => $.notifications.errorMessage),
      })
    },
  })
}
