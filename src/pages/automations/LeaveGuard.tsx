import { useStore } from '@workflowbuilder/sdk'
import { useTranslation } from 'react-i18next'

import { LeaveDialog } from '../../components/LeaveDialog.tsx'
import type { AutomationContent } from './editorSession.ts'

// Asks before the person leaves the editor with changes that were not saved; `dirty` is the
// editor session's. It sits inside <WorkflowBuilder.Root> to read the canvas.
export function LeaveGuard({
  name,
  dirty,
}: {
  name: string
  dirty: (content: AutomationContent) => boolean
}) {
  const nodes = useStore((state) => state.nodes)
  const edges = useStore((state) => state.edges)
  const { t } = useTranslation()

  return (
    <LeaveDialog
      dirty={dirty({ name, graph: { nodes, edges } })}
      message={t(($) => $.automations.leaveMessage)}
    />
  )
}
