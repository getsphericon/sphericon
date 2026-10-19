import { useTranslation } from 'react-i18next'

import {
  siteSetupChecklistDismissMutation,
  siteSetupChecklistGetQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useDismissSetupChecklist dismisses a Workspace's Setup checklist for everyone and refreshes
// the checklist read. Owner and Admin only; the server refuses a Member.
export function useDismissSetupChecklist(slug: string) {
  const { t } = useTranslation()
  return useResourceMutation({
    mutation: siteSetupChecklistDismissMutation(),
    invalidate: [siteSetupChecklistGetQueryKey({ path: { slug } })],
    errorTitle: t(($) => $.setupChecklist.dismissError),
    forbiddenMessage: t(($) => $.setupChecklist.dismissForbidden),
  })
}
