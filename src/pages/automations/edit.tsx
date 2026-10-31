import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import { siteAutomationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { AutomationBuilder } from './AutomationBuilder.tsx'
import { editedVersionKey } from './editedVersion.ts'

export function AutomationEditPage() {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={<PageSkeleton />}
      errorTitle={t(($) => $.alerts.automationLoadErrorTitle)}
    >
      <AutomationEdit />
    </QueryBoundary>
  )
}

function AutomationEdit() {
  const { t } = useTranslation()
  const { workspaceId, automationId } = automationsEditRoute.useParams()

  const automation = useSuspenseQuery(
    siteAutomationsGetOptions({ path: { workspaceId, id: automationId } }),
  ).data

  // A new Automation is created with a draft and Publish only moves it to live, so one of the two
  // is always there.
  const edited = automation.draft ?? automation.live
  if (!edited) {
    return (
      <ApiErrorAlert
        error={null}
        title={t(($) => $.alerts.automationLoadErrorTitle)}
        fallback={t(($) => $.alerts.automationLoadErrorTitle)}
      />
    )
  }

  return (
    <AutomationBuilder
      key={editedVersionKey(automation)}
      workspaceId={workspaceId}
      automation={automation}
      edited={edited}
    />
  )
}
