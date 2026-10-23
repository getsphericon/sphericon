import { AppShell, Badge, Button, Container, Group, Title } from '@mantine/core'
import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router'

import { useCart } from './cart.tsx'
import { pageViewed } from './tracking.ts'

function NavButton({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Button component={NavLink} to={to} variant="subtle">
      {children}
    </Button>
  )
}

// A page view on every load and every route change. The last href is remembered so React's
// StrictMode double effect in development sends one event, not two.
function usePageViews(): void {
  const { pathname, search } = useLocation()
  const href = `${pathname}${search}`
  const last = useRef<string | null>(null)

  useEffect(() => {
    if (last.current === href) {
      return
    }
    last.current = href
    pageViewed()
  }, [href])
}

export function Layout() {
  const cart = useCart()
  usePageViews()

  return (
    <AppShell header={{ height: 60 }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Title order={3}>Brightpath</Title>
          <Group gap="xs">
            <NavButton to="/">Courses</NavButton>
            <NavButton to="/pricing">Pricing</NavButton>
            <NavButton to="/signup">Sign up</NavButton>
            <NavButton to="/cart">
              Cart{' '}
              <Badge ml={4} size="sm" circle>
                {cart.items.length}
              </Badge>
            </NavButton>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Main>
        <Container size="md">
          <Outlet />
        </Container>
      </AppShell.Main>
    </AppShell>
  )
}
