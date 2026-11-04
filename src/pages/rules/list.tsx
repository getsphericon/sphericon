import { Badge, Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { EventActionNames } from '../../components/EventActionName.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteRulesPublishMutation,
  siteRulesDeactivateMutation,
  siteRulesDeleteMutation,
  siteRulesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesDetailRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/index.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { Route as rulesCreateRoute } from '../../routes/workspaces/$workspaceId/rules/new.tsx'
import { withPage } from '../../searchSchemas.ts'

export function RulesListPage() {
  const { t } = useTranslation()
  const { workspaceId } = rulesRoute.useParams()
  const navigate = useNavigate()

  const publish = useResourceMutation({
    mutation: siteRulesPublishMutation(),
    successMessage: t(($) => $.notifications.rulePublished),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })
  const deactivate = useResourceMutation({
    mutation: siteRulesDeactivateMutation(),
    successMessage: t(($) => $.notifications.ruleDeactivated),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })
  const deleteRule = useDeleteResource({
    mutation: siteRulesDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.notifications.ruleDeleted),
    errorTitle: t(($) => $.alerts.ruleDeleteErrorTitle),
    consequence: t(($) => $.confirmations.deleteRule),
  })
  const { page } = rulesRoute.useSearch()
  const routeNavigate = rulesRoute.useNavigate()

  return (
    <Stack flex={1}>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.rules)}</Title>
        <ButtonLink to={rulesCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.rules.addRule)}
        </ButtonLink>
      </Group>

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) => siteRulesListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.ruleLoadErrorTitle)}
        collection={t(($) => $.nav.rules)}
        empty={{
          title: t(($) => $.rules.noRecords),
          action: (
            <ButtonLink variant="default" to={rulesCreateRoute.to} params={{ workspaceId }}>
              {t(($) => $.rules.addRule)}
            </ButtonLink>
          ),
        }}
        onRowClick={(record) =>
          void navigate({ to: rulesDetailRoute.to, params: { workspaceId, ruleId: record.id } })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.rules.nameLabel) },
          {
            // What the Rule reacts to: its live version, else the draft of one never published.
            accessor: 'live',
            title: t(($) => $.rules.triggerColumn),
            render: (record) => (
              <EventActionNames actions={(record.live ?? record.draft)?.triggerEvents ?? []} />
            ),
          },
          {
            accessor: 'status',
            title: t(($) => $.rules.statusLabel),
            render: (record) => (
              <Badge variant="light" color={record.status === 'active' ? 'teal' : 'gray'}>
                {record.status === 'active'
                  ? t(($) => $.rules.statusActive)
                  : t(($) => $.rules.statusInactive)}
              </Badge>
            ),
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={rulesDetailRoute.to}
                    params={{ workspaceId, ruleId: record.id }}
                  >
                    {t(($) => $.actions.view)}
                  </MenuItemLink>
                  {record.status === 'active' ? (
                    <Menu.Item
                      onClick={() => deactivate.mutate({ path: { workspaceId, id: record.id } })}
                    >
                      {t(($) => $.rules.deactivate)}
                    </Menu.Item>
                  ) : (
                    <Menu.Item
                      onClick={() => publish.mutate({ path: { workspaceId, id: record.id } })}
                    >
                      {t(($) => $.rules.publish)}
                    </Menu.Item>
                  )}
                  <MenuItemLink to={rulesEditRoute.to} params={{ workspaceId, ruleId: record.id }}>
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                  <Menu.Item color="red" onClick={() => deleteRule(record.id)}>
                    {t(($) => $.actions.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
