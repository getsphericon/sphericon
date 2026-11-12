import { Button, Modal, Stack } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { docCodec } from '../../utils/messageDoc.ts'
import { propertySetter } from './nodeProperties.ts'
import type { ReadProperties } from './nodeProperties.ts'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The properties panel is narrow, so the message is edited in a dialog opened from it. A step
// keeps its document as JSON text beside the body (the form holds only strings): an edit in the
// editor writes the document, an edit of a message that has none writes the body and clears the
// document, so the step saves one or the other.
export function MessageControl(props: ControlProps) {
  const { data, label } = props
  const { t } = useTranslation()
  const { core } = useJsonForms()
  const [opened, { open, close }] = useDisclosure(false)

  const body = typeof data === 'string' ? data : ''
  const node: ReadProperties<'email'> = isRecord(core?.data) ? core.data : {}
  const docText = node.doc
  const set = propertySetter<'email'>(props)
  const doc = useMemo(() => {
    const parsed = docCodec.safeParse(docText)
    return parsed.success ? parsed.data : null
  }, [docText])

  return (
    <Stack gap="xs">
      <Button variant="default" onClick={open}>
        {t(($) => $.automations.editMessage)}
      </Button>
      <Modal opened={opened} onClose={close} title={label} size="xl">
        <Stack>
          {/* An Automation's message also binds the Event its Enrollment began on (#752). */}
          <MessageEditor
            doc={doc}
            body={body}
            eventTags
            onChange={(edit) => {
              if ('doc' in edit) {
                set('doc', z.encode(docCodec, edit.doc))
              } else {
                set('doc', '')
                set('body', edit.body)
              }
            }}
          />
          <Button onClick={close}>{t(($) => $.automations.messageDone)}</Button>
        </Stack>
      </Modal>
    </Stack>
  )
}
