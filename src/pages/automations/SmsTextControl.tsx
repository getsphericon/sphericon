import type { ControlProps } from '@workflowbuilder/sdk'

import { SmsContentEditor } from '../../components/sms/SmsContentEditor.tsx'
import { EVENT_ACTION_TAG } from '../../utils/mergeTags.ts'

// The Automation step's use of the shared SMS editor: the step's text is the control's own
// property. An Automation's message also binds the Event its Enrollment began on.
export function SmsTextControl(props: ControlProps) {
  const { data, path } = props
  return (
    <SmsContentEditor
      extraTags={[EVENT_ACTION_TAG]}
      value={typeof data === 'string' ? data : ''}
      onChange={(next) => props.handleChange(path, next)}
    />
  )
}
