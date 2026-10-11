import { Avatar, Group, Menu, NavLink, Stack, Text, UnstyledButton } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconChevronDown, IconHome, IconLogout } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMatchRoute, useNavigate, useRouteContext } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { operatorAuthLogoutMutation } from '../generated/operator/@tanstack/react-query.gen.ts'
import { DashboardShell } from '../layouts/DashboardShell.tsx'
import { consoleAuthedRoute, consoleHomeRoute, consoleLoginRoute } from '../router.tsx'

function ConsoleNavbar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const matchRoute = useMatchRoute()

  return (
    <Stack gap="xs">
      <NavLink
        label={t(($) => $.console.nav.workspaces)}
        leftSection={<IconHome size={18} />}
        active={Boolean(matchRoute({ to: consoleHomeRoute.to }))}
        onClick={() => navigate({ to: consoleHomeRoute.to })}
      />
    </Stack>
  )
}

// The header menu: who is signed in, and logout (clears the Operator cookie).
function OperatorMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { operator } = useRouteContext({ from: consoleAuthedRoute.id })

  const logoutMutation = useMutation({
    ...operatorAuthLogoutMutation(),
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: consoleLoginRoute.to })
    },
    onError: () =>
      notifications.show({
        color: 'red',
        title: t(($) => $.console.menu.logoutErrorTitle),
        message: t(($) => $.console.menu.logoutErrorMessage),
      }),
  })

  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <UnstyledButton>
          <Group gap="xs" wrap="nowrap">
            <Avatar radius="xl" size={32} color="blue">
              {operator.email.charAt(0).toUpperCase()}
            </Avatar>
            <Text size="sm" visibleFrom="sm">
              {operator.email}
            </Text>
            <IconChevronDown size={16} />
          </Group>
        </UnstyledButton>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Item
          color="red"
          leftSection={<IconLogout size={16} />}
          onClick={() => logoutMutation.mutate({})}
        >
          {t(($) => $.console.menu.logout)}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

export function ConsoleLayout() {
  return <DashboardShell sidebar={<ConsoleNavbar />} headerRight={<OperatorMenu />} />
}
