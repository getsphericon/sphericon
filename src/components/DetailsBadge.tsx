import { Badge, Popover, Stack, Text } from '@mantine/core'
import type { BadgeProps } from '@mantine/core'
import { getHotkeyHandler, useDisclosure } from '@mantine/hooks'
import { IconInfoCircle } from '@tabler/icons-react'
import type { ReactNode } from 'react'

// DetailsBadge is a short value in a table cell (a status, a count) whose explanation opens in a
// popover when the badge is chosen: why an endpoint is off, which scopes a key has. The cell
// keeps one word; the details stay one click or keypress away, on touch screens too. Without
// details it is a plain badge. The rule is "Status and its details" in docs/standards/ui.md.
export function DetailsBadge({
  details,
  children,
  ...badge
}: Pick<BadgeProps, 'color' | 'variant' | 'size'> & {
  // Each line of the popover, in order; empty or absent lines are dropped.
  details?: readonly ReactNode[]
  children: ReactNode
}) {
  const [opened, { toggle, close }] = useDisclosure(false)
  const lines = (details ?? []).filter((line) => line !== null && line !== undefined && line !== '')
  if (lines.length === 0) {
    return <Badge {...badge}>{children}</Badge>
  }
  // Mantine closes on Escape only from inside the dropdown; the focus stays on the badge, which
  // closes it too.
  return (
    <Popover
      opened={opened}
      onDismiss={close}
      width={320}
      position="bottom-start"
      withArrow
      shadow="md"
    >
      <Popover.Target>
        <Badge
          {...badge}
          component="button"
          type="button"
          onClick={toggle}
          onKeyDown={getHotkeyHandler([['Escape', close]])}
          rightSection={<IconInfoCircle size={12} aria-hidden />}
        >
          {children}
        </Badge>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap={4}>
          {lines.map((line, index) => (
            <Text key={index} size="sm">
              {line}
            </Text>
          ))}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}
