import { AppShell, Badge, Button, Container, Group, Title } from '@mantine/core'
import type { ReactNode } from 'react'
import { NavLink, Outlet } from 'react-router'

import { useCart } from './cartContext.ts'

function NavButton({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Button component={NavLink} to={to} variant="subtle">
      {children}
    </Button>
  )
}

export function Layout() {
  const cart = useCart()

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
