import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesGetOptions,
  siteWorkspacesListQueryKey,
  siteWorkspacesSetConfirmedOptInMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useSetConfirmedOptIn turns a Workspace's confirmed opt-in (ADR 0013) on or off and refreshes the
// Workspace it answers with. Owner and Admin only; the server refuses a Member. A refused switch
// (the Workspace changed since it was read, ADR 0119) still says why, and reads the Workspace again.
export function useSetConfirmedOptIn(workspaceId: string) {
  const { t } = useTranslation()
  const workspaceKeys = [
    siteWorkspacesListQueryKey(),
    siteWorkspacesGetOptions({ path: { workspaceId } }).queryKey,
  ]
  return useResourceMutation({
    mutation: siteWorkspacesSetConfirmedOptInMutation(),
    invalidate: workspaceKeys,
    successMessage: t(($) => $.settings.confirmedOptIn.saved),
    errorTitle: t(($) => $.settings.confirmedOptIn.errorTitle),
    forbiddenMessage: t(($) => $.settings.confirmedOptIn.forbidden),
    rereadOnError: true,
  })
}
