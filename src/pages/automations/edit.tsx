import { useSuspenseQuery } from '@tanstack/react-query'
import { Navigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import { siteAutomationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { AutomationBuilder } from './AutomationBuilder.tsx'

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

  // An archived Automation is read-only until it is unarchived (ADR 0100): its page, which offers
  // no Edit, stands in for the builder.
  if (automation.archivedAt) {
    return (
      <Navigate to={automationsDetailRoute.to} params={{ workspaceId, automationId }} replace />
    )
  }

  // A new Automation is created with a draft and Publish only moves it to live, so one of the two
  // is always there.
  const edited = automation.draft ?? automation.live
  if (!edited) {
    return <ApiErrorAlert error={null} title={t(($) => $.alerts.automationLoadErrorTitle)} />
  }

  return (
    <AutomationBuilder
      key={automation.id}
      workspaceId={workspaceId}
      automation={automation}
      edited={edited}
    />
  )
}
