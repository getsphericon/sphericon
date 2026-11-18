import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesGetOptions,
  siteWorkspacesListQueryKey,
  siteWorkspacesSetConfirmedOptInMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useSetConfirmedOptIn turns a Workspace's confirmed opt-in (ADR 0013) on or off and refreshes the
// Workspace it answers with. Owner and Admin only; the server refuses a Member.
export function useSetConfirmedOptIn(workspaceId: string) {
  const { t } = useTranslation()
  return useResourceMutation({
    mutation: siteWorkspacesSetConfirmedOptInMutation(),
    invalidate: [
      siteWorkspacesListQueryKey(),
      siteWorkspacesGetOptions({ path: { workspaceId } }).queryKey,
    ],
    successMessage: t(($) => $.settings.confirmedOptIn.saved),
    errorTitle: t(($) => $.settings.confirmedOptIn.errorTitle),
    forbiddenMessage: t(($) => $.settings.confirmedOptIn.forbidden),
  })
}
