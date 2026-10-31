import { useStore } from '@workflowbuilder/sdk'
import { I18nextProvider } from 'react-i18next'

import { LeaveDialog } from '../../components/LeaveDialog.tsx'
import { i18n } from '../../i18n.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

// Asks before the person leaves the editor with changes that were not saved. It sits inside
// <WorkflowBuilder.Root> to read the canvas.
export function LeaveGuard({
  name,
  integrationId,
  saved,
}: {
  name: string
  integrationId: string | null
  saved: string
}) {
  const nodes = useStore((state) => state.nodes)
  const edges = useStore((state) => state.edges)

  return (
    <I18nextProvider i18n={i18n}>
      <LeaveDialog
        dirty={automationSnapshot(name, integrationId, { nodes, edges }) !== saved}
        message={i18n.t(($) => $.automations.leaveMessage)}
      />
    </I18nextProvider>
  )
}
