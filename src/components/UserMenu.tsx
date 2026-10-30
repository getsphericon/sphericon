import { Avatar, Box, Group, Menu, Text, UnstyledButton } from '@mantine/core'
import {
  IconChevronDown,
  IconExternalLink,
  IconLogout,
  IconSettings,
  IconUser,
} from '@tabler/icons-react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteUserGetMeOptions,
  siteWorkspacesAccountLinkMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useLogout } from '../hooks/useLogout.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { DeletedWorkspacesMenuSection } from './DeletedWorkspacesMenuSection.tsx'
import { MenuItemLink } from './RouterLink.tsx'

// UserMenu is the header account dropdown: the signed-in User's block (it opens the profile),
// workspace settings, logout.
// `workspaceId` is the active workspace (absent in the account layout); the workspace
// settings item only renders when one is set. Logout is useLogout.
export function UserMenu({
  workspaceId,
  label,
  accountLink = false,
}: {
  workspaceId?: string
  label?: string
  accountLink?: boolean
}) {
  const { t } = useTranslation()
  const notify = useNotify()
  const me = useQuery(siteUserGetMeOptions()).data

  // The Account link: the server mints a short-lived token and names the page; the
  // app only opens it.
  const accountLinkMutation = useMutation({
    ...siteWorkspacesAccountLinkMutation(),
    onSuccess: ({ url, token }) => {
      const target = new URL(url)
      target.searchParams.set('token', token)
      window.open(target.toString(), '_blank', 'noopener')
    },
    onError: () => {
      notify({ color: 'red', message: t(($) => $.userMenu.accountErrorTitle) })
    },
  })

  const logoutMutation = useLogout()

  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <UnstyledButton>
          <Group gap="xs" wrap="nowrap">
            <Avatar radius="xl" size={32} color="blue">
              {label ? label.charAt(0).toUpperCase() : <IconUser size={18} />}
            </Avatar>
            {label ? <Text size="sm">{label}</Text> : null}
            <Box visibleFrom="sm" lh={0}>
              <IconChevronDown size={16} />
            </Box>
          </Group>
        </UnstyledButton>
      </Menu.Target>

      <Menu.Dropdown>
        <MenuItemLink to={profileRoute.to} py="xs">
          <Group gap="sm" wrap="nowrap">
            <Avatar radius="xl" size={36} color="blue">
              {me ? me.name.charAt(0).toUpperCase() : <IconUser size={18} />}
            </Avatar>
            {me ? (
              <div>
                <Text size="sm" fw={500}>
                  {me.name}
                </Text>
                <Text size="xs" c="dimmed">
                  {me.email}
                </Text>
              </div>
            ) : (
              <Text size="sm">{t(($) => $.userMenu.profile)}</Text>
            )}
          </Group>
        </MenuItemLink>
        <Menu.Divider />
        {workspaceId ? (
          <MenuItemLink
            to={settingsRoute.to}
            params={{ workspaceId }}
            leftSection={<IconSettings size={16} />}
          >
            {t(($) => $.userMenu.workspaceSettings)}
          </MenuItemLink>
        ) : null}
        {workspaceId && accountLink ? (
          <Menu.Item
            leftSection={<IconExternalLink size={16} />}
            onClick={() => accountLinkMutation.mutate({ path: { workspaceId } })}
          >
            {t(($) => $.userMenu.account)}
          </Menu.Item>
        ) : null}
        <DeletedWorkspacesMenuSection />
        <Menu.Divider />
        <Menu.Item leftSection={<IconLogout size={16} />} onClick={() => logoutMutation.mutate({})}>
          {t(($) => $.userMenu.logout)}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}
