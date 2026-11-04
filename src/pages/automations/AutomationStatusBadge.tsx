import { useTranslation } from 'react-i18next'

import { DetailsBadge } from '../../components/DetailsBadge.tsx'
import type {
  SiteAutomationResource,
  SiteAutomationStatus,
} from '../../generated/site/types.gen.ts'

// An Automation never published is a draft; once published, its status from the contract.
type AutomationState = 'draft' | SiteAutomationStatus

const STATE_COLORS: Record<AutomationState, string> = {
  draft: 'gray',
  active: 'teal',
  inactive: 'orange',
}

// The one status of an Automation (docs/standards/ui.md, "Status and its details"): Draft until it
// is first published, then Active or Inactive. Unpublished changes above the live version are not
// a status of their own: the automation still runs its live version, so they open as the badge's
// details where `withDraftDetails` asks for them (the list; the detail page has its own controls).
export function AutomationStatusBadge({
  automation,
  withDraftDetails = false,
}: {
  automation: Pick<SiteAutomationResource, 'status' | 'live' | 'draft'>
  withDraftDetails?: boolean
}) {
  const { t } = useTranslation()
  const state: AutomationState = automation.live === null ? 'draft' : automation.status
  const pending = automation.live !== null && automation.draft !== null
  return (
    <DetailsBadge
      variant="light"
      color={STATE_COLORS[state]}
      details={withDraftDetails && pending ? [t(($) => $.automations.pendingDetails)] : []}
    >
      {t(($) => $.automations.state[state])}
    </DetailsBadge>
  )
}
