import { Badge, Button, Group, Loader, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteRuleFiringsListOptions,
  siteRulesActivateMutation,
  siteRulesDeactivateMutation,
  siteRulesGetOptions,
  siteRulesGetQueryKey,
  siteRulesListQueryKey,
  siteRulesUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteRuleActionStatus } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesDetailRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/index.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { formatDateTime } from '../../utils/datetime.ts'

const resultColor = {
  [SiteRuleActionStatus.SUCCEEDED]: 'teal',
  [SiteRuleActionStatus.FAILED]: 'red',
  [SiteRuleActionStatus.SKIPPED]: 'gray',
}

// The Rule with its Firings: each Event that fired it, the Contact and what every Action did.
export function RuleDetailPage() {
  const { t } = useTranslation()
  const { workspaceId, ruleId } = rulesDetailRoute.useParams()
  const ruleQuery = useQuery(siteRulesGetOptions({ path: { workspaceId, id: ruleId } }))

  const invalidate = [
    siteRulesGetQueryKey({ path: { workspaceId, id: ruleId } }),
    siteRulesListQueryKey({ path: { workspaceId } }),
  ]
  const activate = useResourceMutation({
    mutation: siteRulesActivateMutation(),
    invalidate,
    successMessage: t(($) => $.notifications.ruleActivated),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })
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
    return <Loader />
  }
  if (ruleQuery.isError || !ruleQuery.data) {
    return (
      <ApiErrorAlert
        error={ruleQuery.error}
        title={t(($) => $.alerts.ruleLoadErrorTitle)}
        fallback={t(($) => $.alerts.ruleLoadErrorTitle)}
      />
    )
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
              : t(($) => $.rules.statusDraft)}
          </Badge>
        </Group>
        <Group gap="xs">
          {rule.archivedAt ? (
            <Button loading={restore.isPending} onClick={() => restore.mutate({ path })}>
              {t(($) => $.actions.unarchive)}
            </Button>
          ) : (
            <>
              {rule.status === 'active' ? (
                <Button
                  variant="default"
                  loading={deactivate.isPending}
                  onClick={() => deactivate.mutate({ path })}
                >
                  {t(($) => $.rules.deactivate)}
                </Button>
              ) : (
                <Button loading={activate.isPending} onClick={() => activate.mutate({ path })}>
                  {t(($) => $.rules.activate)}
                </Button>
              )}
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

      <Title order={5}>{t(($) => $.rules.firingsTitle)}</Title>
      <PagedTable
        options={(query) => siteRuleFiringsListOptions({ path: { workspaceId, ruleId }, query })}
        loadErrorTitle={t(($) => $.alerts.firingsLoadErrorTitle)}
        noRecordsText={t(($) => $.rules.noFirings)}
        columns={[
          {
            accessor: 'createdAt',
            title: t(($) => $.rules.firingFired),
            render: (record) => formatDateTime(record.createdAt),
          },
          { accessor: 'action', title: t(($) => $.rules.firingAction) },
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
