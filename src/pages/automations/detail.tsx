import { Badge, Group, Loader, Stack, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsGetOptions,
  siteEnrollmentsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { formatDateTime } from '../../utils/datetime.ts'
import { AutomationReport } from './AutomationReport.tsx'

export function AutomationDetailPage() {
  const { t } = useTranslation()
  const { workspaceId, automationId } = automationsDetailRoute.useParams()
  const navigate = useNavigate()
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

  const automation = getQuery.data

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Group gap="sm">
          <Title order={4}>{automation.name}</Title>
          <Badge variant="light" color={automation.status === 'active' ? 'teal' : 'gray'}>
            {automation.status === 'active'
              ? t(($) => $.automations.statusActive)
              : t(($) => $.automations.statusDraft)}
          </Badge>
        </Group>
        <Group gap="xs">
          <ButtonLink
            variant="default"
            to={automationsEditRoute.to}
            params={{ workspaceId, automationId }}
          >
            {t(($) => $.actions.edit)}
          </ButtonLink>
          <ButtonLink variant="subtle" to={automationsRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
        </Group>
      </Group>

      <AutomationReport workspaceId={workspaceId} automationId={automationId} />

      <Title order={5}>{t(($) => $.automations.enrollmentsTitle)}</Title>
      <PagedTable
        options={(query) =>
          siteEnrollmentsListOptions({ path: { workspaceId, automationId }, query })
        }
        loadErrorTitle={t(($) => $.alerts.loadErrorTitle)}
        noRecordsText={t(($) => $.automations.noEnrollments)}
        columns={[
          { accessor: 'contactId', title: t(($) => $.automations.failedEnrollmentContact) },
          {
            accessor: 'status',
            title: t(($) => $.automations.statusLabel),
            render: (record) => t(($) => $.automations.enrollmentStatus[record.status]),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.automations.enrolledAt),
            render: (record) => formatDateTime(record.createdAt),
          },
        ]}
        onRowClick={(record) =>
          void navigate({
            to: contactsDetailRoute.to,
            params: { workspaceId, contactId: record.contactId },
          })
        }
      />
    </Stack>
  )
}
