import {
  ActionIcon,
  type ActionIconProps,
  Anchor,
  type AnchorProps,
  Button,
  type ButtonProps,
  Menu,
  type MenuItemProps,
  NavLink,
  type NavLinkProps,
} from '@mantine/core'
import { createLink, type LinkComponent } from '@tanstack/react-router'
import { forwardRef } from 'react'

// Typed TanStack Router links rendered as Mantine controls. Mantine's
// polymorphic `component={Link}` loses the router's `to`/`params` inference, so
// we wrap the controls with `createLink` (the documented UI-library integration)
// to keep navigation type-safe and using real anchors (preload, middle-click).

type ButtonLinkProps = Omit<ButtonProps, 'href'>

const ButtonLinkBase = forwardRef<HTMLAnchorElement, ButtonLinkProps>((props, ref) => (
  <Button ref={ref} component="a" {...props} />
))

const ButtonLinkCreated = createLink(ButtonLinkBase)

export const ButtonLink: LinkComponent<typeof ButtonLinkBase> = (props) => (
  <ButtonLinkCreated preload="intent" {...props} />
)

type ActionIconLinkProps = Omit<ActionIconProps, 'href'>

const ActionIconLinkBase = forwardRef<HTMLAnchorElement, ActionIconLinkProps>((props, ref) => (
  <ActionIcon ref={ref} component="a" {...props} />
))

const ActionIconLinkCreated = createLink(ActionIconLinkBase)

export const ActionIconLink: LinkComponent<typeof ActionIconLinkBase> = (props) => (
  <ActionIconLinkCreated preload="intent" {...props} />
)

type NavLinkLinkProps = Omit<NavLinkProps, 'href'>

const NavLinkBase = forwardRef<HTMLAnchorElement, NavLinkLinkProps>((props, ref) => (
  <NavLink ref={ref} component="a" {...props} />
))

const NavLinkCreated = createLink(NavLinkBase)

export const NavLinkLink: LinkComponent<typeof NavLinkBase> = (props) => (
  <NavLinkCreated preload="intent" {...props} />
)

type AnchorLinkProps = Omit<AnchorProps, 'href'>

const AnchorLinkBase = forwardRef<HTMLAnchorElement, AnchorLinkProps>((props, ref) => (
  <Anchor ref={ref} component="a" {...props} />
))

const AnchorLinkCreated = createLink(AnchorLinkBase)

export const AnchorLink: LinkComponent<typeof AnchorLinkBase> = (props) => (
  <AnchorLinkCreated preload="intent" {...props} />
)

type MenuItemLinkProps = Omit<MenuItemProps, 'href'>

const MenuItemLinkBase = forwardRef<HTMLAnchorElement, MenuItemLinkProps>((props, ref) => (
  <Menu.Item ref={ref} component="a" {...props} />
))

const MenuItemLinkCreated = createLink(MenuItemLinkBase)

export const MenuItemLink: LinkComponent<typeof MenuItemLinkBase> = (props) => (
  <MenuItemLinkCreated preload="intent" {...props} />
)
