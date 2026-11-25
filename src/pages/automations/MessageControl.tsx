import { Button, Modal, Stack } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useTranslation } from 'react-i18next'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import type { EditorDocument } from '../../generated/site/types.gen.ts'

// The side panel is narrow, so an email step's message is edited in a dialog opened from it. An
// edit in the editor writes the document, an edit of a message that has none writes the body and
// clears the document, so the step saves one or the other.
export function MessageControl({
  doc,
  body,
  onChange,
}: {
  doc: EditorDocument | null
  body: string
  onChange: (message: { doc: EditorDocument | null; body: string }) => void
}) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)

  return (
    <Stack gap="xs">
      <Button variant="default" onClick={open}>
        {t(($) => $.automations.editMessage)}
      </Button>
      <Modal
        opened={opened}
        onClose={close}
        title={t(($) => $.automations.emailBodyLabel)}
        size="xl"
      >
        <Stack>
          {/* An Automation's message also binds the Event its Enrollment began on (#752). */}
          <MessageEditor
            doc={doc}
            body={body}
            eventTags
            onChange={(edit) => {
              onChange('doc' in edit ? { doc: edit.doc, body } : { doc: null, body: edit.body })
            }}
          />
          <Button onClick={close}>{t(($) => $.automations.messageDone)}</Button>
        </Stack>
      </Modal>
    </Stack>
  )
}
