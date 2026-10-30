import { Modal } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import type { ReactNode } from 'react'

// useActionModal holds the inputs of one action (a password, a code, a new email) in a modal:
// `open` goes on the button that starts the action and `modal` is placed beside that button's
// row, never inside it. The content is rendered only while the modal is open, so whatever was
// typed is gone once it closes; content gets the callback that closes it, for Cancel and for a
// success.
export function useActionModal(title: string, content: (close: () => void) => ReactNode) {
  const [opened, { open, close }] = useDisclosure(false)
  const modal = (
    <Modal opened={opened} onClose={close} title={title}>
      {opened ? content(close) : null}
    </Modal>
  )
  return { open, modal }
}
