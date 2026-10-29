import { Anchor, Button, Menu, NavLink, useMantineTheme } from '@mantine/core'
import type { AnchorProps, ButtonProps, MenuItemProps, NavLinkProps } from '@mantine/core'
import { useMediaQuery } from '@mantine/hooks'
import { createLink } from '@tanstack/react-router'
import type { LinkComponent } from '@tanstack/react-router'
import { forwardRef } from 'react'

// Typed TanStack Router links rendered as Mantine controls. Mantine's
// polymorphic `component={Link}` loses the router's `to`/`params` inference, so
// we wrap the controls with `createLink` (the documented UI-library integration)
// to keep navigation type-safe and using real anchors (preload, middle-click).

type ButtonLinkProps = Omit<ButtonProps, 'href'>

const ButtonLinkBase = forwardRef<HTMLAnchorElement, ButtonLinkProps>(
  function ButtonLinkBase(props, ref) {
    return <Button ref={ref} component="a" {...props} />
  },
)

const ButtonLinkCreated = createLink(ButtonLinkBase)

export const ButtonLink: LinkComponent<typeof ButtonLinkBase> = (props) => (
  <ButtonLinkCreated preload="intent" {...props} />
)

type NavLinkLinkProps = Omit<NavLinkProps, 'href'>

const NavLinkBase = forwardRef<HTMLAnchorElement, NavLinkLinkProps>(
  function NavLinkBase(props, ref) {
    return <NavLink ref={ref} component="a" {...props} />
  },
)

const NavLinkCreated = createLink(NavLinkBase)

// A navbar link on a phone is a link in the drawer, which slides out while the page changes: the
// router's cross-fade of the whole viewport would fade the old snapshot of the open drawer under
// the sliding live one. So there the page swaps without it; the sidebar of a wide screen keeps it.
export const NavLinkLink: LinkComponent<typeof NavLinkBase> = (props) => {
  const theme = useMantineTheme()
  const drawer = useMediaQuery(`(max-width: ${theme.breakpoints.sm})`)
  return <NavLinkCreated preload="intent" viewTransition={!drawer} {...props} />
}

type AnchorLinkProps = Omit<AnchorProps, 'href'>

const AnchorLinkBase = forwardRef<HTMLAnchorElement, AnchorLinkProps>(
  function AnchorLinkBase(props, ref) {
    return <Anchor ref={ref} component="a" {...props} />
  },
)

const AnchorLinkCreated = createLink(AnchorLinkBase)

export const AnchorLink: LinkComponent<typeof AnchorLinkBase> = (props) => (
  <AnchorLinkCreated preload="intent" {...props} />
)

type MenuItemLinkProps = Omit<MenuItemProps, 'href'>

const MenuItemLinkBase = forwardRef<HTMLAnchorElement, MenuItemLinkProps>(
  function MenuItemLinkBase(props, ref) {
    return <Menu.Item ref={ref} component="a" {...props} />
  },
)

const MenuItemLinkCreated = createLink(MenuItemLinkBase)

export const MenuItemLink: LinkComponent<typeof MenuItemLinkBase> = (props) => (
  <MenuItemLinkCreated preload="intent" {...props} />
)
