import { Button, Modal, Stack } from '@mantine/core'
import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { docCodec } from '../../utils/messageDoc.ts'

// The properties panel is narrow, so the message is edited in a dialog opened from it. A step
// keeps its document as JSON text beside the body (the form holds only strings): an edit in the
// editor writes the document, an edit of a message that has none writes the body and clears the
// document, so the step saves one or the other.
export function MessageControl(props: ControlProps) {
  const { data, label } = props
  const { t } = useTranslation()
  const { core } = useJsonForms()
  const [opened, setOpened] = useState(false)

  const body = typeof data === 'string' ? data : ''
  const docText: unknown = core?.data?.doc
  const doc = useMemo(() => {
    const parsed = docCodec.safeParse(docText)
    return parsed.success ? parsed.data : null
  }, [docText])

  return (
    <Stack gap="xs">
      <Button variant="default" onClick={() => setOpened(true)}>
        {t(($) => $.automations.editMessage)}
      </Button>
      <Modal opened={opened} onClose={() => setOpened(false)} title={label} size="xl">
        <Stack>
          {/* An Automation's message also binds the Event its Enrollment began on (#752). */}
          <MessageEditor
            doc={doc}
            body={body}
            eventTags
            onChange={(edit) => {
              if ('doc' in edit) {
                props.handleChange('doc', z.encode(docCodec, edit.doc))
              } else {
                props.handleChange('doc', '')
                props.handleChange('body', edit.body)
              }
            }}
          />
          <Button onClick={() => setOpened(false)}>{t(($) => $.automations.messageDone)}</Button>
        </Stack>
      </Modal>
    </Stack>
  )
}
