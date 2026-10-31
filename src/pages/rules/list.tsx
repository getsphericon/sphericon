import { Badge, Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteRulesPublishMutation,
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
import { withPage } from '../../searchSchemas.ts'

export function RulesListPage() {
  const { t } = useTranslation()
  const { workspaceId } = rulesRoute.useParams()
  const navigate = useNavigate()

  const listQueryKey = siteRulesListQueryKey({ path: { workspaceId } })
  const publish = useResourceMutation({
    mutation: siteRulesPublishMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.rulePublished),
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
  const { archived = false, page } = rulesRoute.useSearch()
  const routeNavigate = rulesRoute.useNavigate()

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.rules)}</Title>
        <ButtonLink to={rulesCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.rules.addRule)}
        </ButtonLink>
      </Group>

      <ArchiveViewControl
        archived={archived}
        onChange={(next) =>
          void routeNavigate({
            search: { archived: next || undefined },
            replace: true,
            viewTransition: false,
          })
        }
      />

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
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
            // What the Rule reacts to: its live version, else the draft of one never published.
            accessor: 'live',
            title: t(($) => $.rules.triggerColumn),
            render: (record) => (record.live ?? record.draft)?.triggerEvents.join(', '),
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
                          onClick={() => publish.mutate({ path: { workspaceId, id: record.id } })}
                        >
                          {t(($) => $.rules.publish)}
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
