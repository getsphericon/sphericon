import { Button, Group, Modal, Text } from '@mantine/core'
import { useBlocker } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

// LeaveDialog asks before the person leaves a page that holds changes not saved yet: a link, Back
// or a closed tab. `message` names what would be lost.
export function LeaveDialog({ dirty, message }: { dirty: boolean; message: string }) {
  const { t } = useTranslation()
  const blocker = useBlocker({
    shouldBlockFn: () => dirty,
    enableBeforeUnload: dirty,
    withResolver: true,
  })

  return (
    <Modal
      opened={blocker.status === 'blocked'}
      onClose={() => blocker.reset?.()}
      title={t(($) => $.leave.title)}
    >
      <Text mb="md">{message}</Text>
      <Group justify="flex-end">
        <Button variant="default" onClick={() => blocker.reset?.()}>
          {t(($) => $.leave.stay)}
        </Button>
        <Button color="red" onClick={() => blocker.proceed?.()}>
          {t(($) => $.leave.discard)}
        </Button>
      </Group>
    </Modal>
  )
}
