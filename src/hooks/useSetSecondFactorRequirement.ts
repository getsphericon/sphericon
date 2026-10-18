import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListQueryKey,
  siteWorkspacesSetSecondFactorRequirementMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useSetSecondFactorRequirement switches a Workspace's Two-factor requirement (ADR
// 0020) and refreshes the Workspace list, which carries the requirement and the
// signed-in User's grace. Owner and Admin only; the server refuses a Member.
export function useSetSecondFactorRequirement() {
  const { t } = useTranslation()
  return useResourceMutation({
    mutation: siteWorkspacesSetSecondFactorRequirementMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.secondFactorRequirement.saved),
    errorTitle: t(($) => $.secondFactorRequirement.errorTitle),
    forbiddenMessage: t(($) => $.secondFactorRequirement.forbidden),
  })
}
