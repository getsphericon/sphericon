import { Button, Modal, Stack } from '@mantine/core'
import { formatIs, rankWith, useJsonForms, withJsonFormsControlProps } from '@workflowbuilder/sdk'
import type { ControlProps, JsonFormsRendererExtension } from '@workflowbuilder/sdk'
import { useMemo, useState } from 'react'
import { I18nextProvider, useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { i18n } from '../../i18n.ts'
import { docCodec } from '../../utils/messageDoc.ts'

// The schema `format` that marks a property as the message of an email step.
export const MESSAGE_FORMAT = 'message'

// The properties panel is narrow, so the message is edited in a dialog opened from it. A step
// keeps its document as JSON text beside the body (the form holds only strings): an edit in the
// editor writes the document, an edit of a message that has none writes the body and clears the
// document, so the step saves one or the other.
function MessageControl(props: ControlProps) {
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
          <MessageEditor
            doc={doc}
            body={body}
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

// The builder subtree reads the global i18next instance (see AutomationBuilder); the dialog is
// the app's own UI, so it is bound back to the app's instance.
function MessageControlInApp(props: ControlProps) {
  return (
    <I18nextProvider i18n={i18n}>
      <MessageControl {...props} />
    </I18nextProvider>
  )
}

// A stable reference: the SDK registers the renderers it is given.
export const messageRenderer: JsonFormsRendererExtension = {
  tester: rankWith(100, formatIs(MESSAGE_FORMAT)),
  renderer: withJsonFormsControlProps(MessageControlInApp),
}
