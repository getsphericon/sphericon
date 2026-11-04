import { Button, Group, Menu, Stack } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useNavigate } from '@tanstack/react-router'
import type { MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { draftState } from '../../components/draftState.ts'
import { EventActionName } from '../../components/EventActionName.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { AnchorLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteAutomationsPublishMutation,
  siteAutomationsArchiveMutation,
  siteAutomationsDeactivateMutation,
  siteAutomationsDeleteMutation,
  siteAutomationsListOptions,
  siteAutomationsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { messageRevenueCells } from '../../utils/messageRevenue.ts'
import { AutomationStatusBadge } from './AutomationStatusBadge.tsx'
import { AutomationCreateModal } from './create.tsx'

export function AutomationsListPage() {
  const { t } = useTranslation()
  const { locale } = useFormat()
  const { workspaceId } = automationsRoute.useParams()
  const navigate = useNavigate()

  const deleteAutomation = useDeleteResource({
    mutation: siteAutomationsDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.notifications.automationDeleted),
    errorTitle: t(($) => $.alerts.automationDeleteErrorTitle),
    consequence: t(($) => $.confirmations.deleteAutomation),
  })

  const publishMutation = useResourceMutation({
    mutation: siteAutomationsPublishMutation(),
    successMessage: t(($) => $.notifications.automationPublished),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const deactivateMutation = useResourceMutation({
    mutation: siteAutomationsDeactivateMutation(),
    successMessage: t(($) => $.notifications.automationDeactivated),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const archiveMutation = useResourceMutation({
    mutation: siteAutomationsArchiveMutation(),
    successMessage: t(($) => $.notifications.automationArchived),
    errorTitle: t(($) => $.alerts.automationArchiveErrorTitle),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteAutomationsUnarchiveMutation(),
    successMessage: t(($) => $.notifications.automationRestored),
    errorTitle: t(($) => $.alerts.automationRestoreErrorTitle),
  })

  const { archived = false, page } = automationsRoute.useSearch()
  const routeNavigate = automationsRoute.useNavigate()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)

  // A row's actions in the order of docs/standards/ui.md: view, edit, the others, delete. Publish
  // and Deactivate follow the detail page's DraftControls (draftState), so an active automation
  // with unpublished changes offers both. Archive is undone by Restore, so only Delete is red.
  const rowActions = (record: SiteAutomationResource) => {
    const path = { workspaceId, id: record.id }
    if (record.archivedAt) {
      return (
        <Menu.Item onClick={() => restoreMutation.mutate({ path })}>
          {t(($) => $.actions.unarchive)}
        </Menu.Item>
      )
    }
    const { publishable } = draftState({
      active: record.status === 'active',
      hasLive: record.live !== null,
      hasDraft: record.draft !== null,
      dirty: false,
    })
    const params = { workspaceId, automationId: record.id }
    return (
      <>
        <MenuItemLink to={automationsDetailRoute.to} params={params}>
          {t(($) => $.actions.view)}
        </MenuItemLink>
        <MenuItemLink to={automationsEditRoute.to} params={params}>
          {t(($) => $.actions.edit)}
        </MenuItemLink>
        {publishable ? (
          <Menu.Item onClick={() => publishMutation.mutate({ path })}>
            {t(($) => $.automations.publish)}
          </Menu.Item>
        ) : null}
        {record.status === 'active' ? (
          <Menu.Item onClick={() => deactivateMutation.mutate({ path })}>
            {t(($) => $.automations.deactivate)}
          </Menu.Item>
        ) : null}
        <Menu.Item onClick={() => archiveMutation.mutate({ path })}>
          {t(($) => $.actions.archive)}
        </Menu.Item>
        <Menu.Item color="red" onClick={() => deleteAutomation(record.id)}>
          {t(($) => $.actions.delete)}
        </Menu.Item>
      </>
    )
  }

  return (
    <Stack flex={1}>
      <AutomationCreateModal opened={createOpened} onClose={closeCreate} />
      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) =>
          siteAutomationsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.automationLoadErrorTitle)}
        collection={t(($) => $.nav.automations)}
        pageHeader={{
          actions: <Button onClick={openCreate}>{t(($) => $.automations.addAutomation)}</Button>,
          count: (total) => t(($) => $.automations.count, { count: total }),
          toolbar: (
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
          ),
        }}
        // The archived view is another place, not the list to create in: its empty state invites
        // nothing (docs/standards/ui.md, "Empty states").
        empty={
          archived
            ? { title: t(($) => $.automations.noArchivedRecords) }
            : {
                title: t(($) => $.automations.noRecords),
                action: (
                  <Button variant="default" onClick={openCreate}>
                    {t(($) => $.automations.addAutomation)}
                  </Button>
                ),
              }
        }
        onRowClick={(record) =>
          void navigate({
            to: automationsDetailRoute.to,
            params: { workspaceId, automationId: record.id },
          })
        }
        columns={[
          {
            accessor: 'name',
            title: t(($) => $.automations.nameLabel),
            // The name is the link to the automation, so it is reachable by keyboard and opens in
            // a new tab; it looks like the row's text, not underlined even on hover, since the whole row opens it. The link keeps
            // the row's own click from navigating a second time.
            render: (record) => (
              <AnchorLink
                inherit
                c="inherit"
                fw={500}
                underline="never"
                to={automationsDetailRoute.to}
                params={{ workspaceId, automationId: record.id }}
                onClick={(event: MouseEvent) => event.stopPropagation()}
              >
                {record.name}
              </AnchorLink>
            ),
          },
          {
            accessor: 'status',
            title: t(($) => $.automations.statusLabel),
            render: (record) => <AutomationStatusBadge automation={record} withDraftDetails />,
          },
          {
            accessor: 'triggerEvent',
            title: t(($) => $.automations.triggerColumn),
            // What enrolls Contacts: the live version's Trigger, or the draft's before publishing.
            render: (record) => {
              const trigger = (record.live ?? record.draft)?.triggerEvent
              return trigger === undefined ? '-' : <EventActionName action={trigger} />
            },
          },
          {
            accessor: 'orders',
            title: t(($) => $.messageRevenue.orders),
            textAlign: 'right',
            render: (record) => messageRevenueCells(record.revenue, locale).orders,
          },
          {
            accessor: 'revenue',
            title: t(($) => $.messageRevenue.revenue),
            textAlign: 'right',
            render: (record) => messageRevenueCells(record.revenue, locale).revenue,
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>{rowActions(record)}</RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
