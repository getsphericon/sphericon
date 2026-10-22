import { Badge, Group, Menu, Stack, Switch, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteRulesActivateMutation,
  siteRulesArchiveMutation,
  siteRulesDeactivateMutation,
  siteRulesDeleteMutation,
  siteRulesListOptions,
  siteRulesListQueryKey,
  siteRulesUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
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

  const archive = useResourceMutation({
    mutation: siteRulesArchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.ruleArchived),
    errorTitle: t(($) => $.alerts.ruleArchiveErrorTitle),
  })
  const restore = useResourceMutation({
    mutation: siteRulesUnarchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.ruleRestored),
    errorTitle: t(($) => $.alerts.ruleRestoreErrorTitle),
  })
  const deleteRule = useDeleteResource({
    mutation: siteRulesDeleteMutation(),
    workspaceId,
    listQueryKey,
    successMessage: t(($) => $.notifications.ruleDeleted),
    errorTitle: t(($) => $.alerts.ruleDeleteErrorTitle),
  })
  const [archived, setArchived] = useState(false)

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.rules)}</Title>
        <ButtonLink to={rulesCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.rules.addRule)}
        </ButtonLink>
      </Group>

      <Switch
        label={t(($) => $.rules.showArchived)}
        checked={archived}
        onChange={(event) => setArchived(event.currentTarget.checked)}
      />

      <PagedTable
        key={String(archived)}
        options={(query) =>
          siteRulesListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
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
                  {record.archivedAt ? (
                    <Menu.Item
                      onClick={() => restore.mutate({ path: { workspaceId, id: record.id } })}
                    >
                      {t(($) => $.actions.unarchive)}
                    </Menu.Item>
                  ) : (
                    [
                      <MenuItemLink
                        key="view"
                        to={rulesDetailRoute.to}
                        params={{ workspaceId, ruleId: record.id }}
                      >
                        {t(($) => $.actions.view)}
                      </MenuItemLink>,
                      record.status === 'active' ? (
                        <Menu.Item
                          key="toggle"
                          onClick={() =>
                            deactivate.mutate({ path: { workspaceId, id: record.id } })
                          }
                        >
                          {t(($) => $.rules.deactivate)}
                        </Menu.Item>
                      ) : (
                        <Menu.Item
                          key="toggle"
                          onClick={() => activate.mutate({ path: { workspaceId, id: record.id } })}
                        >
                          {t(($) => $.rules.activate)}
                        </Menu.Item>
                      ),
                      <MenuItemLink
                        key="edit"
                        to={rulesEditRoute.to}
                        params={{ workspaceId, ruleId: record.id }}
                      >
                        {t(($) => $.actions.edit)}
                      </MenuItemLink>,
                      <Menu.Item
                        key="archive"
                        color="red"
                        onClick={() => archive.mutate({ path: { workspaceId, id: record.id } })}
                      >
                        {t(($) => $.actions.archive)}
                      </Menu.Item>,
                      <Menu.Item key="delete" color="red" onClick={() => deleteRule(record.id)}>
                        {t(($) => $.actions.delete)}
                      </Menu.Item>,
                    ]
                  )}
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
