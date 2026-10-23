import { Group, Menu, Text } from '@mantine/core'
import { IconRestore } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListDeletedOptions,
  siteWorkspacesListDeletedQueryKey,
  siteWorkspacesListQueryKey,
  siteWorkspacesRestoreMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { formatDate } from '../utils/datetime.ts'

// DeletedWorkspacesMenuSection lists the Workspaces the User owns that are in their
// deletion window, with the date their data is erased, and restores one (ADR 0043). It
// renders inside the open menu, so the list is fetched when the menu opens.
export function DeletedWorkspacesMenuSection() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const deleted = useQuery(siteWorkspacesListDeletedOptions())
  const restoreMutation = useResourceMutation({
    mutation: siteWorkspacesRestoreMutation(),
    invalidate: [siteWorkspacesListQueryKey(), siteWorkspacesListDeletedQueryKey()],
    successMessage: t(($) => $.userMenu.restored),
    errorTitle: t(($) => $.userMenu.restoreErrorTitle),
    onDone: async (restored) =>
      navigate({ to: overviewRoute.to, params: { workspaceId: restored.id } }),
  })

  const items = deleted.data ?? []
  if (items.length === 0) {
    return null
  }

  return (
    <>
      <Menu.Divider />
      <Menu.Label>{t(($) => $.userMenu.deletedWorkspaces)}</Menu.Label>
      {items.map((workspace) => (
        <Menu.Item
          key={workspace.id}
          closeMenuOnClick={false}
          leftSection={<IconRestore size={16} />}
          aria-label={t(($) => $.userMenu.restore, { name: workspace.name })}
          disabled={restoreMutation.isPending}
          onClick={() => restoreMutation.mutate({ path: { workspaceId: workspace.id } })}
        >
          <Group gap="xs" wrap="nowrap">
            <Text size="sm">{workspace.name}</Text>
            <Text size="xs" c="dimmed">
              {t(($) => $.userMenu.erasedOn, { date: formatDate(workspace.eraseAt) })}
            </Text>
          </Group>
        </Menu.Item>
      ))}
    </>
  )
}
