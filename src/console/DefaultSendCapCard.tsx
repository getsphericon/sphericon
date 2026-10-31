import { Alert, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import {
  staffInstanceClearDefaultSendCapMutation,
  staffInstanceSetDefaultSendCapMutation,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import type { StaffDefaultSendCap } from '../generated/staff/types.gen.ts'
import { SendCapCard } from './SendCapCard.tsx'

// The warning the Instance page shows while no default Send cap is set: every new Workspace then
// starts uncapped.
export function NoDefaultSendCapAlert() {
  const { t } = useTranslation()
  return (
    <Alert color="yellow" title={t(($) => $.console.instance.defaultSendCap.missingTitle)}>
      <Text size="sm">{t(($) => $.console.instance.defaultSendCap.missing)}</Text>
    </Alert>
  )
}

// The default Send cap (ADR 0067): what every Workspace created from now on starts with. A Staff
// member sets, changes or removes it; existing Workspaces keep their own. The page shows this card
// only on an Instance that has Send caps.
export function DefaultSendCapCard({ defaultSendCap }: { defaultSendCap: StaffDefaultSendCap }) {
  const { t } = useTranslation()
  const perDay = defaultSendCap.perDay ?? null

  return (
    <SendCapCard
      perDay={perDay}
      copy={{
        title: t(($) => $.console.instance.defaultSendCap.title),
        summary:
          perDay === null
            ? t(($) => $.console.instance.defaultSendCap.none)
            : t(($) => $.console.instance.defaultSendCap.value, { perDay }),
        description: t(($) => $.console.instance.defaultSendCap.description),
        set: t(($) => $.console.instance.defaultSendCap.set),
        change: t(($) => $.console.instance.defaultSendCap.change),
        remove: t(($) => $.console.instance.defaultSendCap.remove),
        removeTitle: t(($) => $.console.instance.defaultSendCap.removeTitle),
        removeDescription: t(($) => $.console.instance.defaultSendCap.removeDescription),
        formTitle: t(($) => $.console.instance.defaultSendCap.formTitle),
        formDescription: t(($) => $.console.instance.defaultSendCap.formDescription),
        saved: t(($) => $.console.instance.defaultSendCap.saved),
        removed: t(($) => $.console.instance.defaultSendCap.removed),
        unchanged: t(($) => $.console.instance.defaultSendCap.unchanged),
        error: t(($) => $.console.instance.defaultSendCap.error),
      }}
      save={staffInstanceSetDefaultSendCapMutation()}
      saveVariables={(body) => ({ body })}
      clear={staffInstanceClearDefaultSendCapMutation()}
      clearVariables={{}}
    />
  )
}
