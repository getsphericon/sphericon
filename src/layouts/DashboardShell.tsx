import { AppShell, Box, Burger, Button, Group, ScrollArea, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconBook } from '@tabler/icons-react'
import { Outlet, useLocation } from '@tanstack/react-router'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ThemeToggle } from '../components/ThemeToggle.tsx'
import { DOCS_URL } from '../utils/docs.ts'

// DashboardShell is the common authenticated chrome shared by the
// workspace-scoped and account (workspace-independent) layouts: the brand
// header with a right-side slot, a left sidebar slot, and the routed main area.
// On narrow viewports the sidebar collapses behind a burger; it reopens per
// navigation and auto-closes whenever the route changes.
export function DashboardShell({
  sidebar,
  headerRight,
  banner,
  content,
}: {
  sidebar: ReactNode
  headerRight?: ReactNode
  /** A notice shown above every routed page (e.g. a workspace suspension). */
  banner?: ReactNode
  /** Shown instead of the routed page (e.g. a Workspace withheld by its Two-factor requirement). */
  content?: ReactNode
}) {
  const { t } = useTranslation()
  const [opened, { toggle, close }] = useDisclosure(false)
  const location = useLocation()

  // Close the mobile drawer after navigating so the overlay never lingers
  // (derived during render, the documented alternative to setState in an effect).
  const [drawerPath, setDrawerPath] = useState(location.pathname)
  if (drawerPath !== location.pathname) {
    setDrawerPath(location.pathname)
    close()
  }

  return (
    <AppShell
      header={{ height: 64 }}
      navbar={{ width: 260, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
      <AppShell.Header>
        <Group h="100%" px={{ base: 'xs', sm: 'md' }} justify="space-between" wrap="nowrap">
          <Group gap="sm" wrap="nowrap">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Title order={3}>{t(($) => $.app.name)}</Title>
            <Button
              component="a"
              href={DOCS_URL}
              target="_blank"
              rel="noreferrer"
              variant="subtle"
              visibleFrom="sm"
              leftSection={<IconBook size={18} />}
            >
              {t(($) => $.nav.docs)}
            </Button>
          </Group>
          <Group gap="sm" wrap="nowrap" flex={1} miw={0} justify="flex-end">
            {headerRight}
            <ThemeToggle />
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar>
        <AppShell.Section grow component={ScrollArea}>
          <Box p="md">{sidebar}</Box>
        </AppShell.Section>
      </AppShell.Navbar>

      <AppShell.Main>
        {banner}
        {content ?? <Outlet />}
      </AppShell.Main>
    </AppShell>
  )
}
