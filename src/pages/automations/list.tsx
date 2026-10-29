import { Badge, Button, Group, Menu, Stack, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteAutomationsActivateMutation,
  siteAutomationsArchiveMutation,
  siteAutomationsDeactivateMutation,
  siteAutomationsDeleteMutation,
  siteAutomationsListOptions,
  siteAutomationsListQueryKey,
  siteAutomationsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { messageRevenueCells } from '../../utils/messageRevenue.ts'
import { AutomationCreateModal } from './create.tsx'

export function AutomationsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = automationsRoute.useParams()
  const navigate = useNavigate()

  const listQueryKey = siteAutomationsListQueryKey({ path: { workspaceId } })

  const deleteAutomation = useDeleteResource({
    mutation: siteAutomationsDeleteMutation(),
    workspaceId,
    listQueryKey,
    successMessage: t(($) => $.notifications.automationDeleted),
    errorTitle: t(($) => $.alerts.automationDeleteErrorTitle),
  })

  const activateMutation = useResourceMutation({
    mutation: siteAutomationsActivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationActivated),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const deactivateMutation = useResourceMutation({
    mutation: siteAutomationsDeactivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationDeactivated),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const archiveMutation = useResourceMutation({
    mutation: siteAutomationsArchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationArchived),
    errorTitle: t(($) => $.alerts.automationArchiveErrorTitle),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteAutomationsUnarchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationRestored),
    errorTitle: t(($) => $.alerts.automationRestoreErrorTitle),
  })

  const { archived = false } = automationsRoute.useSearch()
  const routeNavigate = automationsRoute.useNavigate()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)

  return (
    <Stack>
      <AutomationCreateModal opened={createOpened} onClose={closeCreate} />
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.automations)}</Title>
        <Button onClick={openCreate}>{t(($) => $.automations.addAutomation)}</Button>
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
        resetKey={String(archived)}
        options={(query) =>
          siteAutomationsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.automationLoadErrorTitle)}
        noRecordsText={t(($) => $.automations.noRecords)}
        emptyState={t(($) => $.automations.emptyState)}
        onRowClick={(record) =>
          void navigate({
            to: automationsDetailRoute.to,
            params: { workspaceId, automationId: record.id },
          })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.automations.nameLabel) },
          { accessor: 'triggerEvent', title: t(($) => $.automations.triggerColumn) },
          {
            accessor: 'status',
            title: t(($) => $.automations.statusLabel),
            render: (record) => (
              <Badge variant="light" color={record.status === 'active' ? 'teal' : 'gray'}>
                {record.status === 'active'
                  ? t(($) => $.automations.statusActive)
                  : t(($) => $.automations.statusDraft)}
              </Badge>
            ),
          },
          {
            accessor: 'orders',
            title: t(($) => $.messageRevenue.orders),
            render: (record) => messageRevenueCells(record.revenue).orders,
          },
          {
            accessor: 'revenue',
            title: t(($) => $.messageRevenue.revenue),
            render: (record) => messageRevenueCells(record.revenue).revenue,
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  {record.archivedAt ? (
                    <Menu.Item
                      onClick={() =>
                        restoreMutation.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.unarchive)}
                    </Menu.Item>
                  ) : (
                    [
                      <MenuItemLink
                        key="view"
                        to={automationsDetailRoute.to}
                        params={{ workspaceId, automationId: record.id }}
                      >
                        {t(($) => $.actions.view)}
                      </MenuItemLink>,
                      record.status === 'active' ? (
                        <Menu.Item
                          key="toggle"
                          onClick={() =>
                            deactivateMutation.mutate({ path: { workspaceId, id: record.id } })
                          }
                        >
                          {t(($) => $.automations.deactivate)}
                        </Menu.Item>
                      ) : (
                        <Menu.Item
                          key="toggle"
                          onClick={() =>
                            activateMutation.mutate({ path: { workspaceId, id: record.id } })
                          }
                        >
                          {t(($) => $.automations.activate)}
                        </Menu.Item>
                      ),
                      <MenuItemLink
                        key="edit"
                        to={automationsEditRoute.to}
                        params={{ workspaceId, automationId: record.id }}
                      >
                        {t(($) => $.actions.edit)}
                      </MenuItemLink>,
                      <Menu.Item
                        key="archive"
                        color="red"
                        onClick={() =>
                          archiveMutation.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.actions.archive)}
                      </Menu.Item>,
                      <Menu.Item
                        key="delete"
                        color="red"
                        onClick={() => deleteAutomation(record.id)}
                      >
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
