import { ActionIcon, Box, Menu } from '@mantine/core'
import { IconDotsVertical } from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

// RowActionsMenu is the three-dots button at the end of a table row; `children` are its
// Menu.Items (destructive ones last, colored red). A click here never reaches the row's own
// click handler (React events bubble out of the dropdown's portal too).
export function RowActionsMenu({ children }: { children: ReactNode }) {
  const { t } = useTranslation()

  return (
    <Box onClick={(event) => event.stopPropagation()}>
      <Menu position="bottom-end" withinPortal>
        <Menu.Target>
          <ActionIcon variant="subtle" color="gray" aria-label={t(($) => $.actions.more)}>
            <IconDotsVertical size={18} />
          </ActionIcon>
        </Menu.Target>
        <Menu.Dropdown>{children}</Menu.Dropdown>
      </Menu>
    </Box>
  )
}
