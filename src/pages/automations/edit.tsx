import { Loader } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteAutomationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { AutomationBuilder } from './AutomationBuilder.tsx'

export function AutomationEditPage() {
  const { t } = useTranslation()
  const { workspaceId, automationId } = automationsEditRoute.useParams()

  const getQuery = useQuery(siteAutomationsGetOptions({ path: { workspaceId, id: automationId } }))

  if (getQuery.isLoading) {
    return <Loader />
  }

  if (getQuery.isError || !getQuery.data) {
    return (
      <ApiErrorAlert
        error={getQuery.error}
        title={t(($) => $.alerts.automationLoadErrorTitle)}
        fallback={t(($) => $.alerts.automationLoadErrorTitle)}
      />
    )
  }

  return <AutomationBuilder workspaceId={workspaceId} automation={getQuery.data} />
}
