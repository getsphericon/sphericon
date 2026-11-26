import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesGetOptions,
  siteWorkspacesListQueryKey,
  siteWorkspacesSetEngagementTrackingDefaultMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useSetEngagementTrackingDefault changes a Workspace's Engagement tracking default (ADR 0120) and
// refreshes the Workspace it answers with. Owner and Admin only; the server refuses a Member. A
// refused change (the Workspace changed since it was read, ADR 0119) still says why, and reads the
// Workspace again.
export function useSetEngagementTrackingDefault(workspaceId: string) {
  const { t } = useTranslation()
  const workspaceKeys = [
    siteWorkspacesListQueryKey(),
    siteWorkspacesGetOptions({ path: { workspaceId } }).queryKey,
  ]
  return useResourceMutation({
    mutation: siteWorkspacesSetEngagementTrackingDefaultMutation(),
    invalidate: workspaceKeys,
    successMessage: t(($) => $.settings.engagementTrackingDefault.saved),
    errorTitle: t(($) => $.settings.engagementTrackingDefault.errorTitle),
    forbiddenMessage: t(($) => $.settings.engagementTrackingDefault.forbidden),
    rereadOnError: true,
  })
}
