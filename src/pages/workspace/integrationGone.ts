import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteIntegrationsListQueryKey } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { hasErrorStatus } from '../../utils/apiErrors.ts'

// useIntegrationGone handles a write from an Integration's window that answers 404: someone
// deleted the Integration while the window was open. It says so in a toast under the write's
// error title, closes the window and reloads the list, which no longer has it. Any other error is
// left to the write's own toast. It returns the `handleError` of useResourceMutation for a title.
export function useIntegrationGone(workspaceId: string, onClose: () => void) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const notify = useNotify()
  return (errorTitle: string) => async (error: unknown) => {
    if (!hasErrorStatus(error, [404])) {
      return false
    }
    notify({ color: 'red', title: errorTitle, message: t(($) => $.settings.integrations.gone) })
    onClose()
    await queryClient.invalidateQueries({
      queryKey: siteIntegrationsListQueryKey({ path: { workspaceId } }),
    })
    return true
  }
}
