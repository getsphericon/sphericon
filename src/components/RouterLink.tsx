import { ActionIcon, Button } from '@mantine/core'
import type { ActionIconProps, ButtonProps } from '@mantine/core'
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

type ActionIconLinkProps = Omit<ActionIconProps, 'href'>

const ActionIconLinkBase = forwardRef<HTMLAnchorElement, ActionIconLinkProps>(
  function ActionIconLinkBase(props, ref) {
    return <ActionIcon ref={ref} component="a" {...props} />
  },
)

const ActionIconLinkCreated = createLink(ActionIconLinkBase)

export const ActionIconLink: LinkComponent<typeof ActionIconLinkBase> = (props) => (
  <ActionIconLinkCreated preload="intent" {...props} />
)
