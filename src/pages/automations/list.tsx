import { Button, Group, Menu, Select, Stack, TextInput } from '@mantine/core'
import { useDebouncedCallback, useDisclosure } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import type { MouseEvent, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { draftState } from '../../components/draftState.ts'
import { EventActionName } from '../../components/EventActionName.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { RelativeTime } from '../../components/RelativeTime.tsx'
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
import { zSiteAutomationState } from '../../generated/site/zod.gen.ts'
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

// The direction a column sorts in until the person turns it: by name A to Z, by creation the
// newest first.
const defaultDirection = (column: string) => (column === 'name' ? 'asc' : 'desc')

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

  const { archived = false, page, name, state, sort, direction } = automationsRoute.useSearch()
  const routeNavigate = automationsRoute.useNavigate()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)

  // The search field follows the URL when Back, forward or a link changes it, unless it already
  // shows that search (typing trims before it navigates).
  const [search, setSearch] = useState(name ?? '')
  const [seenName, setSeenName] = useState(name)
  if (name !== seenName) {
    setSeenName(name)
    if (search.trim() !== (name ?? '')) {
      setSearch(name ?? '')
    }
  }
  // A change of the view, a filter or the order changes only its own keys of the search and
  // starts again from the first page.
  type ListSearch = ReturnType<typeof automationsRoute.useSearch>
  const changeSearch = (patch: Partial<ListSearch>) =>
    void routeNavigate({
      search: (prev) => ({ ...prev, ...patch, page: undefined }),
      replace: true,
      viewTransition: false,
    })
  const applySearch = useDebouncedCallback((value: string) => {
    changeSearch({ name: value.trim() || undefined })
  }, 300)
  const filtered = Boolean(name || state)
  const clearFilters = () => {
    setSearch('')
    changeSearch({ name: undefined, state: undefined })
  }

  // The order on screen: the newest first unless the URL says otherwise, and by name A to Z
  // unless it says Z to A. The default order is the absent params, so the plain URL is the list.
  const column = sort ?? 'createdAt'
  // A column picked anew starts in its own direction; the column already sorted by turns round.
  const changeSort = (nextColumn: string, nextDirection: 'asc' | 'desc') => {
    const nextSort = nextColumn === 'name' ? 'name' : 'createdAt'
    const turned = nextSort === column ? nextDirection : defaultDirection(nextSort)
    changeSearch({
      sort: nextSort === 'createdAt' ? undefined : nextSort,
      direction: turned === defaultDirection(nextSort) ? undefined : turned,
    })
  }

  // A search or filter that matches nothing offers to clear it, not to create the first
  // automation; the archived view is another place, not the list to create in, so its empty state
  // invites nothing (docs/standards/ui.md, "Empty states").
  let empty: { title: string; action?: ReactNode } = {
    title: t(($) => $.automations.noRecords),
    action: (
      <Button variant="default" onClick={openCreate}>
        {t(($) => $.automations.addAutomation)}
      </Button>
    ),
  }
  if (filtered) {
    empty = {
      title: t(($) => $.automations.noMatches),
      action: (
        <Button variant="default" onClick={clearFilters}>
          {t(($) => $.table.clearFilter)}
        </Button>
      ),
    }
  } else if (archived) {
    empty = { title: t(($) => $.automations.noArchivedRecords) }
  }

  const listQuery = {
    archived,
    ...(name && { name }),
    ...(state && { state }),
    ...(sort && { sort }),
    ...(direction && { direction }),
  }

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
          siteAutomationsListOptions({ path: { workspaceId }, query: { ...query, ...listQuery } })
        }
        loadErrorTitle={t(($) => $.alerts.automationLoadErrorTitle)}
        collection={t(($) => $.nav.automations)}
        pageHeader={{
          actions: <Button onClick={openCreate}>{t(($) => $.automations.addAutomation)}</Button>,
          count: (total) => t(($) => $.automations.count, { count: total }),
          toolbar: (
            <>
              <ArchiveViewControl
                archived={archived}
                onChange={(next) => changeSearch({ archived: next || undefined })}
              />
              <TextInput
                w={240}
                value={search}
                onChange={(event) => {
                  setSearch(event.currentTarget.value)
                  applySearch(event.currentTarget.value)
                }}
                placeholder={t(($) => $.automations.searchPlaceholder)}
                aria-label={t(($) => $.automations.searchPlaceholder)}
                leftSection={<IconSearch size={16} />}
              />
              <Select
                w={180}
                clearable
                aria-label={t(($) => $.automations.stateFilter)}
                placeholder={t(($) => $.automations.stateAll)}
                data={zSiteAutomationState.options.map((value) => ({
                  value,
                  label: t(($) => $.automations.state[value]),
                }))}
                value={state ?? null}
                onChange={(next) =>
                  changeSearch({
                    state: zSiteAutomationState
                      .optional()
                      .catch(undefined)
                      .parse(next ?? undefined),
                  })
                }
              />
            </>
          ),
        }}
        sort={{
          column,
          direction: direction ?? defaultDirection(column),
          onChange: changeSort,
        }}
        empty={empty}
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
            sortable: true,
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
            accessor: 'createdAt',
            title: t(($) => $.automations.createdLabel),
            sortable: true,
            render: (record) => <RelativeTime value={record.createdAt} />,
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
