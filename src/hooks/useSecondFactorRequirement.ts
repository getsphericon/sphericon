import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListQueryKey,
  siteWorkspacesLiftSecondFactorRequirementMutation,
  siteWorkspacesRequireSecondFactorMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useRequireSecondFactor switches a Workspace's Two-factor requirement (ADR 0020) on and
// refreshes the Workspace list, which carries the requirement and the signed-in User's grace.
// Owner and Admin only; the server refuses a Member.
export function useRequireSecondFactor() {
  return useResourceMutation({
    mutation: siteWorkspacesRequireSecondFactorMutation(),
    ...useRequirementFeedback(),
  })
}

// useLiftSecondFactorRequirement switches the requirement off: the one operation a User the
// requirement withholds may still perform in the Workspace.
export function useLiftSecondFactorRequirement() {
  return useResourceMutation({
    mutation: siteWorkspacesLiftSecondFactorRequirementMutation(),
    ...useRequirementFeedback(),
  })
}

// useRequirementFeedback is what both switches refresh and tell the User.
function useRequirementFeedback() {
  const { t } = useTranslation()
  return {
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.secondFactorRequirement.saved),
    errorTitle: t(($) => $.secondFactorRequirement.errorTitle),
    forbiddenMessage: t(($) => $.secondFactorRequirement.forbidden),
  }
}
