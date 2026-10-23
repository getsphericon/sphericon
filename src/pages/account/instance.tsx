import { Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { TelemetrySection } from '../../components/TelemetrySection.tsx'
import { useInstanceState } from '../../hooks/useInstanceState.ts'

// InstancePage is where an Owner of the oldest Workspace decides about the Instance itself.
export function InstancePage() {
  const { t } = useTranslation()
  const state = useInstanceState()

  return (
    <Stack maw={640}>
      <Title order={2}>{t(($) => $.instance.title)}</Title>
      {state.isPending ? <BlockSkeleton /> : null}
      {state.data ? <TelemetrySection state={state.data} /> : null}
      {state.isError ? <Text c="dimmed">{t(($) => $.instance.ownersOnly)}</Text> : null}
    </Stack>
  )
}
