import { Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import {
  staffWorkspacesClearSendCapMutation,
  staffWorkspacesSetSendCapMutation,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffSendCap } from '../generated/staffee/types.gen.ts'
import { SendCapCard } from './SendCapCard.tsx'

// A Workspace's Send cap (ADR 0067): the most messages it may send per rolling 24 hours across
// all its Integrations, with its sends of the last 24 hours. A Staff member sets, changes or
// removes it; the Workspace only reads it. The page shows this card only on an Instance that has
// Send caps.
export function WorkspaceSendCapCard({
  workspaceId,
  sendCap,
}: {
  workspaceId: string
  sendCap: StaffSendCap
}) {
  const { t } = useTranslation()
  const perDay = sendCap.perDay ?? null
  const sent = sendCap.sentLast24h

  return (
    <SendCapCard
      perDay={perDay}
      copy={{
        title: t(($) => $.console.workspace.sendCap.title),
        summary:
          perDay === null
            ? t(($) => $.console.workspace.sendCap.none)
            : t(($) => $.console.workspace.sendCap.value, { perDay }),
        description: t(($) => $.console.workspace.sendCap.description),
        set: t(($) => $.console.workspace.sendCap.set),
        change: t(($) => $.console.workspace.sendCap.change),
        remove: t(($) => $.console.workspace.sendCap.remove),
        removeTitle: t(($) => $.console.workspace.sendCap.removeTitle),
        removeDescription: t(($) => $.console.workspace.sendCap.removeDescription),
        formTitle: t(($) => $.console.workspace.sendCap.formTitle),
        formDescription: t(($) => $.console.workspace.sendCap.formDescription),
        saved: t(($) => $.console.workspace.sendCap.saved),
        removed: t(($) => $.console.workspace.sendCap.removed),
        unchanged: t(($) => $.console.workspace.sendCap.unchanged),
        error: t(($) => $.console.workspace.sendCap.error),
      }}
      save={staffWorkspacesSetSendCapMutation()}
      saveVariables={(body) => ({ path: { workspaceId }, body })}
      clear={staffWorkspacesClearSendCapMutation()}
      clearVariables={{ path: { workspaceId } }}
    >
      <Text size="sm">
        {perDay === null
          ? t(($) => $.console.workspace.sendCap.sent, { sent })
          : t(($) => $.console.workspace.sendCap.sentOf, { sent, perDay })}
      </Text>
    </SendCapCard>
  )
}
