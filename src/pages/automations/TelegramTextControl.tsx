import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'

import { TelegramContentEditor } from '../../components/telegram/TelegramContentEditor.tsx'
import { TelegramUnsubscribeControl } from '../../components/telegram/TelegramUnsubscribeControl.tsx'
import { EVENT_ACTION_TAG } from '../../utils/mergeTags.ts'
import { buttonsCodec } from '../../utils/telegramButtons.ts'
import { OFF, propertySetter } from './nodeProperties.ts'
import type { ReadProperties } from './nodeProperties.ts'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The Automation step's use of the shared Telegram editor. The step's text is the control's own
// property; its image, buttons (held as JSON text: the form holds only strings) and link-preview
// switch and unsubscribe button are its siblings on the node, written back beside it. An Automation's message also binds
// the Event its Enrollment began on.
export function TelegramTextControl(props: ControlProps) {
  const { data } = props
  const { core } = useJsonForms()
  const node: ReadProperties<'telegram'> = isRecord(core?.data) ? core.data : {}
  const set = propertySetter<'telegram'>(props)

  return (
    <TelegramContentEditor
      extraTags={[EVENT_ACTION_TAG]}
      value={{
        text: typeof data === 'string' ? data : '',
        imageUrl: typeof node.imageUrl === 'string' ? node.imageUrl : '',
        buttons: buttonsCodec.read(node.buttons),
        linkPreview: node.linkPreview !== OFF,
      }}
      onChange={(next) => {
        props.handleChange(props.path, next.text)
        set('imageUrl', next.imageUrl)
        set('buttons', buttonsCodec.write(next.buttons))
        set('linkPreview', next.linkPreview ? '' : OFF)
      }}
    >
      <TelegramUnsubscribeControl
        label={typeof node.unsubscribeButton === 'string' ? node.unsubscribeButton : ''}
        onChange={(label) => set('unsubscribeButton', label)}
      />
    </TelegramContentEditor>
  )
}
