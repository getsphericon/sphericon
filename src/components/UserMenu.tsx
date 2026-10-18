import { Avatar, Group, Menu, Text, UnstyledButton } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  IconChevronDown,
  IconExternalLink,
  IconLogout,
  IconSettings,
  IconUser,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteAuthLogoutMutation,
  siteWorkspacesAccountLinkMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as loginRoute } from '../routes/login.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$slug/settings.tsx'

// UserMenu is the header account dropdown: profile, workspace settings, logout.
// `slug` is the active workspace (absent in the account layout); the workspace
// settings item only renders when one is set. Logout calls the site logout
// operation, which clears the session cookie; we then drop cached queries and
// route to login.
export function UserMenu({
  slug,
  label,
  accountLink = false,
}: {
  slug?: string
  label?: string
  accountLink?: boolean
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

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
      notifications.show({ color: 'red', message: t(($) => $.userMenu.accountErrorTitle) })
    },
  })

  const logoutMutation = useMutation({
    ...siteAuthLogoutMutation(),
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: loginRoute.to })
    },
    onError: () => {
      notifications.show({
        color: 'red',
        title: t(($) => $.userMenu.logoutErrorTitle),
        message: t(($) => $.userMenu.logoutErrorTitle),
      })
    },
  })

  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <UnstyledButton>
          <Group gap="xs">
            <Avatar radius="xl" size={32} color="blue">
              {label ? label.charAt(0).toUpperCase() : <IconUser size={18} />}
            </Avatar>
            {label ? <Text size="sm">{label}</Text> : null}
            <IconChevronDown size={16} />
          </Group>
        </UnstyledButton>
      </Menu.Target>

      <Menu.Dropdown>
        <Menu.Item
          leftSection={<IconUser size={16} />}
          onClick={() => void navigate({ to: profileRoute.to })}
        >
          {t(($) => $.userMenu.profile)}
        </Menu.Item>
        {slug ? (
          <Menu.Item
            leftSection={<IconSettings size={16} />}
            onClick={() => void navigate({ to: settingsRoute.to, params: { slug } })}
          >
            {t(($) => $.userMenu.workspaceSettings)}
          </Menu.Item>
        ) : null}
        {slug && accountLink ? (
          <Menu.Item
            leftSection={<IconExternalLink size={16} />}
            onClick={() => accountLinkMutation.mutate({ path: { slug } })}
          >
            {t(($) => $.userMenu.account)}
          </Menu.Item>
        ) : null}
        <Menu.Divider />
        <Menu.Item
          color="red"
          leftSection={<IconLogout size={16} />}
          onClick={() => logoutMutation.mutate({})}
        >
          {t(($) => $.userMenu.logout)}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}
