import { Badge, Group, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { DraftControls } from '../../components/DraftControls.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteAutomationsGetOptions,
  siteEnrollmentsListOptions,
  siteIntegrationsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteAutomationStepType } from '../../generated/site/types.gen.ts'
import type { SiteAutomationStep, SiteAutomationVersion } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { formatDateTime } from '../../utils/datetime.ts'
import { AutomationReport } from './AutomationReport.tsx'
import { automationDraftActions } from './draftActions.ts'

// The Integration a version sends through, by name (its id until the name loads).
function IntegrationName({ workspaceId, id }: { workspaceId: string; id: string }) {
  const query = useQuery(siteIntegrationsGetOptions({ path: { workspaceId, id } }))
  return <Text size="sm">{query.data?.name ?? id}</Text>
}

// A step by its kind, with the subject of an email or the Tag of a tag step.
function stepLabel(t: TFunction, step: SiteAutomationStep): string {
  const kind = {
    [SiteAutomationStepType.EMAIL]: t(($) => $.automations.stepEmail),
    [SiteAutomationStepType.WAIT]: t(($) => $.automations.stepWait),
    [SiteAutomationStepType.APPLY_TAG]: t(($) => $.automations.stepApplyTag),
    [SiteAutomationStepType.REMOVE_TAG]: t(($) => $.automations.stepRemoveTag),
    [SiteAutomationStepType.HTTP]: t(($) => $.automations.stepHttp),
  }[step.type]
  const detail = step.type === SiteAutomationStepType.EMAIL ? step.subject : step.tag
  return detail ? `${kind}: ${detail}` : kind
}

// What the live version does: its number, its Trigger, its exit and goal events, its Integration
// and its steps in order. The editor shows the draft when there is one, so this is where the live
// version is read (ADR 0065).
function AutomationVersionSummary({
  workspaceId,
  version,
}: {
  workspaceId: string
  version: SiteAutomationVersion
}) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={5}>{t(($) => $.automations.liveVersion, { number: version.number })}</Title>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.triggerColumn)}
        </Text>
        <Badge variant="outline" color="gray">
          {version.triggerEvent}
        </Badge>
      </Group>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.exitEventsLabel)}
        </Text>
        {version.exitEvents.length === 0 ? (
          <Text size="sm">{t(($) => $.automations.noEndEvent)}</Text>
        ) : (
          version.exitEvents.map((event) => (
            <Badge key={event} variant="outline" color="gray">
              {event}
            </Badge>
          ))
        )}
      </Group>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.goalEventLabel)}
        </Text>
        {version.goalEvent === null ? (
          <Text size="sm">{t(($) => $.automations.noEndEvent)}</Text>
        ) : (
          <Badge variant="outline" color="teal">
            {version.goalEvent}
          </Badge>
        )}
      </Group>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.integrationLabel)}
        </Text>
        {version.integrationId ? (
          <IntegrationName workspaceId={workspaceId} id={version.integrationId} />
        ) : (
          <Text size="sm">{t(($) => $.automations.integrationDefault)}</Text>
        )}
      </Group>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.stepsLabel)}
        </Text>
        {version.steps.map((step, index) => (
          <Badge key={index} variant="light">
            {stepLabel(t, step)}
          </Badge>
        ))}
      </Group>
    </Stack>
  )
}

export function AutomationDetailPage() {
  const { t } = useTranslation()
  const { workspaceId, automationId } = automationsDetailRoute.useParams()
  const navigate = useNavigate()
  const getQuery = useQuery(siteAutomationsGetOptions({ path: { workspaceId, id: automationId } }))

  if (getQuery.isLoading) {
    return <PageSkeleton />
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
              : t(($) => $.automations.statusInactive)}
          </Badge>
        </Group>
        <Group gap="xs">
          {automation.archivedAt ? null : (
            <DraftControls
              workspaceId={workspaceId}
              resource={automation}
              {...automationDraftActions(t, workspaceId, automation.id)}
            />
          )}
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

      {automation.live ? (
        <AutomationVersionSummary workspaceId={workspaceId} version={automation.live} />
      ) : null}

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
