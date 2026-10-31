import { appRenderer } from './appRenderer.ts'
import { MessageControl } from './MessageControl.tsx'
import { MESSAGE_FORMAT } from './messageFormat.ts'

export const messageRenderer = appRenderer(MESSAGE_FORMAT, MessageControl)
