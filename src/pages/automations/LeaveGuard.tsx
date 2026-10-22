import { Button, Group, Modal, Text } from '@mantine/core'
import { useBlocker } from '@tanstack/react-router'
import { useStore } from '@workflowbuilder/sdk'
import { I18nextProvider, useTranslation } from 'react-i18next'

import { i18n } from '../../i18n.ts'
import { graphToSteps } from './definition.ts'
import type { AutomationGraph } from './definition.ts'

// What Save would send, as text: the editor is unsaved while this differs from the saved one.
export function automationSnapshot(name: string, graph: AutomationGraph): string {
  return JSON.stringify([name.trim(), graphToSteps(graph)])
}

function LeaveDialog({ dirty }: { dirty: boolean }) {
  const { t } = useTranslation()
  const blocker = useBlocker({
    shouldBlockFn: () => dirty,
    enableBeforeUnload: dirty,
    withResolver: true,
  })

  return (
    <Modal
      opened={blocker.status === 'blocked'}
      onClose={() => blocker.reset?.()}
      title={t(($) => $.automations.leaveTitle)}
    >
      <Text mb="md">{t(($) => $.automations.leaveMessage)}</Text>
      <Group justify="flex-end">
        <Button variant="default" onClick={() => blocker.reset?.()}>
          {t(($) => $.automations.leaveStay)}
        </Button>
        <Button color="red" onClick={() => blocker.proceed?.()}>
          {t(($) => $.automations.leaveDiscard)}
        </Button>
      </Group>
    </Modal>
  )
}

// Asks before the person leaves the editor with changes that were not saved. It sits inside
// <WorkflowBuilder.Root> to read the canvas.
export function LeaveGuard({ name, saved }: { name: string; saved: string }) {
  const nodes = useStore((state) => state.nodes)
  const edges = useStore((state) => state.edges)

  return (
    <I18nextProvider i18n={i18n}>
      <LeaveDialog dirty={automationSnapshot(name, { nodes, edges }) !== saved} />
    </I18nextProvider>
  )
}
