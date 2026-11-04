import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'

import { SmsContentEditor } from '../../components/sms/SmsContentEditor.tsx'
import { EVENT_ACTION_TAG } from '../../utils/mergeTags.ts'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The Automation step's use of the shared SMS editor: the step's text is the control's own
// property, and its link-tracking switch its sibling on the node ('off' when the links stay as
// written), written back beside it. An Automation's message also binds the Event its Enrollment
// began on.
export function SmsTextControl(props: ControlProps) {
  const { data, path } = props
  const { core } = useJsonForms()
  const node: Record<string, unknown> = isRecord(core?.data) ? core.data : {}
  return (
    <SmsContentEditor
      extraTags={[EVENT_ACTION_TAG]}
      value={{
        text: typeof data === 'string' ? data : '',
        linkTracking: node.linkTracking !== 'off',
      }}
      onChange={(next) => {
        props.handleChange(path, next.text)
        props.handleChange('linkTracking', next.linkTracking ? '' : 'off')
      }}
    />
  )
}
