import { Avatar, Group, Menu, Stack, Text, UnstyledButton } from '@mantine/core'
import {
  IconBuildings,
  IconChevronDown,
  IconHome,
  IconListCheck,
  IconLogout,
  IconServer,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useRouteContext } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { NavLinkLink } from '../components/RouterLink.tsx'
import { staffAuthLogoutMutation } from '../generated/staff/@tanstack/react-query.gen.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { DashboardShell } from '../layouts/DashboardShell.tsx'
import { Route as consoleWorkspacesRoute } from '../routes/console/_authed/_enterprise/workspaces.index.tsx'
import { Route as consoleHomeRoute } from '../routes/console/_authed/index.tsx'
import { Route as consoleInstanceRoute } from '../routes/console/_authed/instance.tsx'
import { Route as consoleAuthedRoute } from '../routes/console/_authed/route.tsx'
import { Route as consoleSignupItemsRoute } from '../routes/console/_authed/signup-items.index.tsx'
import { Route as consoleLoginRoute } from '../routes/console/login.tsx'
import { InstanceNotice } from './InstanceNotice.tsx'

// The sections: the core ones on every Instance, the Enterprise ones only when the server serves
// them (the `staff` license, ADR 0063). A console with them opens on the Workspaces, so it has no
// home of its own to link to.
function ConsoleNavbar() {
  const { t } = useTranslation()
  const { enterprise } = useRouteContext({ from: consoleAuthedRoute.id })

  return (
    <Stack gap="xs">
      {enterprise ? null : (
        <NavLinkLink
          to={consoleHomeRoute.to}
          label={t(($) => $.console.nav.home)}
          leftSection={<IconHome size={18} />}
          activeOptions={{ exact: true }}
        />
      )}
      <NavLinkLink
        to={consoleSignupItemsRoute.to}
        label={t(($) => $.console.nav.signupItems)}
        leftSection={<IconListCheck size={18} />}
      />
      <NavLinkLink
        to={consoleInstanceRoute.to}
        label={t(($) => $.console.nav.instance)}
        leftSection={<IconServer size={18} />}
      />
      {enterprise && (
        <NavLinkLink
          to={consoleWorkspacesRoute.to}
          label={t(($) => $.console.nav.workspaces)}
          leftSection={<IconBuildings size={18} />}
        />
      )}
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
  return (
    <DashboardShell
      sidebar={<ConsoleNavbar />}
      headerRight={<StaffMenu />}
      banner={<InstanceNotice />}
    />
  )
}
