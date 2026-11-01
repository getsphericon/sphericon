import { Badge, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { DraftControls } from '../../components/DraftControls.tsx'
import { EventActionBadge, EventActionName } from '../../components/EventActionName.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteRuleFiringsListOptions,
  siteRulesDeactivateMutation,
  siteRulesGetOptions,
  siteRulesGetQueryKey,
  siteRulesListQueryKey,
  siteRulesUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteRuleActionStatus } from '../../generated/site/types.gen.ts'
import type { SiteRuleVersion } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesDetailRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/index.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { ruleDraftActions } from './draftActions.ts'

const resultColor = {
  [SiteRuleActionStatus.SUCCEEDED]: 'teal',
  [SiteRuleActionStatus.FAILED]: 'red',
  [SiteRuleActionStatus.SKIPPED]: 'gray',
  [SiteRuleActionStatus.PENDING]: 'blue',
}

// What the live version does: its number, its Trigger events and its Actions in order.
function RuleVersionSummary({ version }: { version: SiteRuleVersion }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={5}>{t(($) => $.rules.liveVersion, { number: version.number })}</Title>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.rules.triggerEventsLabel)}
        </Text>
        {version.triggerEvents.map((event) => (
          <EventActionBadge key={event} action={event} color="gray" />
        ))}
      </Group>
      <Group gap="xs">
        <Text size="sm" c="dimmed">
          {t(($) => $.rules.actionsLabel)}
        </Text>
        {version.actions.map((action, index) => (
          <Badge key={index} variant="light">
            {action.tag
              ? `${t(($) => $.rules.actionType[action.type])}: ${action.tag}`
              : t(($) => $.rules.actionType[action.type])}
          </Badge>
        ))}
      </Group>
    </Stack>
  )
}

// The Rule with its live version and its Firings: each Event that fired it, the Contact and what
// every Action did. Its draft is published or reverted here (ADR 0065).
export function RuleDetailPage() {
  const { formatDateTime } = useDateFormat()
  const { page } = rulesDetailRoute.useSearch()
  const pageNavigate = rulesDetailRoute.useNavigate()
  const { t } = useTranslation()
  const { workspaceId, ruleId } = rulesDetailRoute.useParams()
  const ruleQuery = useQuery(siteRulesGetOptions({ path: { workspaceId, id: ruleId } }))

  const invalidate = [
    siteRulesGetQueryKey({ path: { workspaceId, id: ruleId } }),
    siteRulesListQueryKey({ path: { workspaceId } }),
  ]
  const deactivate = useResourceMutation({
    mutation: siteRulesDeactivateMutation(),
    invalidate,
    successMessage: t(($) => $.notifications.ruleDeactivated),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })

  const restore = useResourceMutation({
    mutation: siteRulesUnarchiveMutation(),
    invalidate,
    successMessage: t(($) => $.notifications.ruleRestored),
    errorTitle: t(($) => $.alerts.ruleRestoreErrorTitle),
  })

  if (ruleQuery.isLoading) {
    return <PageSkeleton />
  }
  if (ruleQuery.isError || !ruleQuery.data) {
    return <ApiErrorAlert error={ruleQuery.error} title={t(($) => $.alerts.ruleLoadErrorTitle)} />
  }
  const rule = ruleQuery.data
  const path = { workspaceId, id: ruleId }

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Group>
          <Title order={4}>{rule.name}</Title>
          <Badge variant="light" color={rule.status === 'active' ? 'teal' : 'gray'}>
            {rule.status === 'active'
              ? t(($) => $.rules.statusActive)
              : t(($) => $.rules.statusInactive)}
          </Badge>
        </Group>
        <Group gap="xs">
          {rule.archivedAt ? (
            <Button loading={restore.isPending} onClick={() => restore.mutate({ path })}>
              {t(($) => $.actions.unarchive)}
            </Button>
          ) : (
            <>
              <DraftControls workspaceId={workspaceId} resource={rule} {...ruleDraftActions(t)} />
              {rule.status === 'active' ? (
                <Button
                  variant="default"
                  loading={deactivate.isPending}
                  onClick={() => deactivate.mutate({ path })}
                >
                  {t(($) => $.rules.deactivate)}
                </Button>
              ) : null}
              <ButtonLink variant="default" to={rulesEditRoute.to} params={{ workspaceId, ruleId }}>
                {t(($) => $.actions.edit)}
              </ButtonLink>
            </>
          )}
          <ButtonLink variant="subtle" to={rulesRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
        </Group>
      </Group>

      {rule.live ? <RuleVersionSummary version={rule.live} /> : null}

      <Title order={5}>{t(($) => $.rules.firingsTitle)}</Title>
      <PagedTable
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) => siteRuleFiringsListOptions({ path: { workspaceId, ruleId }, query })}
        loadErrorTitle={t(($) => $.alerts.firingsLoadErrorTitle)}
        noRecordsText={t(($) => $.rules.noFirings)}
        columns={[
          {
            accessor: 'createdAt',
            title: t(($) => $.rules.firingFired),
            render: (record) => formatDateTime(record.createdAt),
          },
          {
            accessor: 'action',
            title: t(($) => $.rules.firingAction),
            render: (record) => <EventActionName action={record.action} />,
          },
          {
            accessor: 'contactId',
            title: t(($) => $.rules.firingContact),
            render: (record) => (
              <ButtonLink
                variant="subtle"
                size="compact-sm"
                to={contactsDetailRoute.to}
                params={{ workspaceId, contactId: record.contactId }}
              >
                {record.contactId}
              </ButtonLink>
            ),
          },
          {
            accessor: 'results',
            title: t(($) => $.rules.firingActions),
            render: (record) => (
              <Stack gap={4}>
                {record.results.map((result, index) => (
                  <Group key={index} gap="xs">
                    <Text size="sm">{t(($) => $.rules.actionType[result.type])}</Text>
                    <Badge variant="light" color={resultColor[result.status]}>
                      {t(($) => $.rules.firingStatus[result.status])}
                    </Badge>
                    {result.detail ? (
                      <Text size="xs" c="dimmed">
                        {result.detail}
                      </Text>
                    ) : null}
                  </Group>
                ))}
                {record.completedAt ? null : (
                  <Badge variant="outline" color="gray">
                    {t(($) => $.rules.firingInProgress)}
                  </Badge>
                )}
              </Stack>
            ),
          },
        ]}
      />
    </Stack>
  )
}
