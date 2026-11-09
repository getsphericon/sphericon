import { useStore } from '@workflowbuilder/sdk'
import { I18nextProvider } from 'react-i18next'

import { LeaveDialog } from '../../components/LeaveDialog.tsx'
import { i18n } from '../../i18n.ts'
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

  return (
    <I18nextProvider i18n={i18n}>
      <LeaveDialog
        dirty={dirty({ name, graph: { nodes, edges } })}
        message={i18n.t(($) => $.automations.leaveMessage)}
      />
    </I18nextProvider>
  )
}
