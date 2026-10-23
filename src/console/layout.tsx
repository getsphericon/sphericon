import { Avatar, Group, Menu, Stack, Text, UnstyledButton } from '@mantine/core'
import { IconChevronDown, IconHome, IconLogout } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useRouteContext } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { NavLinkLink } from '../components/RouterLink.tsx'
import { staffAuthLogoutMutation } from '../generated/staff/@tanstack/react-query.gen.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { DashboardShell } from '../layouts/DashboardShell.tsx'
import { Route as consoleHomeRoute } from '../routes/console/_authed/index.tsx'
import { Route as consoleAuthedRoute } from '../routes/console/_authed/route.tsx'
import { Route as consoleLoginRoute } from '../routes/console/login.tsx'

function ConsoleNavbar() {
  const { t } = useTranslation()

  return (
    <Stack gap="xs">
      <NavLinkLink
        to={consoleHomeRoute.to}
        label={t(($) => $.console.nav.workspaces)}
        leftSection={<IconHome size={18} />}
        activeOptions={{ exact: true }}
      />
    </Stack>
  )
}

// The header menu: who is signed in, and logout (clears the Staff member cookie).
function StaffMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const notify = useNotify()
  const { staff } = useRouteContext({ from: consoleAuthedRoute.id })

  const logoutMutation = useMutation({
    ...staffAuthLogoutMutation(),
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: consoleLoginRoute.to })
    },
    onError: () =>
      notify({
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
              {staff.email.charAt(0).toUpperCase()}
            </Avatar>
            <Text size="sm" visibleFrom="sm">
              {staff.email}
            </Text>
            <IconChevronDown size={16} />
          </Group>
        </UnstyledButton>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Item leftSection={<IconLogout size={16} />} onClick={() => logoutMutation.mutate({})}>
          {t(($) => $.console.menu.logout)}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

export function ConsoleLayout() {
  return <DashboardShell sidebar={<ConsoleNavbar />} headerRight={<StaffMenu />} />
}
