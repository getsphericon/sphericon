import { Badge, Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteRulesActivateMutation,
  siteRulesDeactivateMutation,
  siteRulesListOptions,
  siteRulesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesDetailRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/index.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { Route as rulesCreateRoute } from '../../routes/workspaces/$workspaceId/rules/new.tsx'

export function RulesListPage() {
  const { t } = useTranslation()
  const { workspaceId } = rulesRoute.useParams()
  const navigate = useNavigate()

  const listQueryKey = siteRulesListQueryKey({ path: { workspaceId } })
  const activate = useResourceMutation({
    mutation: siteRulesActivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.ruleActivated),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })
  const deactivate = useResourceMutation({
    mutation: siteRulesDeactivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.ruleDeactivated),
    errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.rules)}</Title>
        <ButtonLink to={rulesCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.rules.addRule)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteRulesListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.ruleLoadErrorTitle)}
        noRecordsText={t(($) => $.rules.noRecords)}
        emptyState={t(($) => $.rules.emptyState)}
        onRowClick={(record) =>
          void navigate({ to: rulesDetailRoute.to, params: { workspaceId, ruleId: record.id } })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.rules.nameLabel) },
          {
            accessor: 'triggerEvents',
            title: t(($) => $.rules.triggerColumn),
            render: (record) => record.triggerEvents.join(', '),
          },
          {
            accessor: 'status',
            title: t(($) => $.rules.statusLabel),
            render: (record) => (
              <Badge variant="light" color={record.status === 'active' ? 'teal' : 'gray'}>
                {record.status === 'active'
                  ? t(($) => $.rules.statusActive)
                  : t(($) => $.rules.statusDraft)}
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
                      onClick={() => activate.mutate({ path: { workspaceId, id: record.id } })}
                    >
                      {t(($) => $.rules.activate)}
                    </Menu.Item>
                  )}
                  <MenuItemLink to={rulesEditRoute.to} params={{ workspaceId, ruleId: record.id }}>
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
