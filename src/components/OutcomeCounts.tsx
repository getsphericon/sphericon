import { SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { DeferralCauseCount, SkippedReasonCount } from '../generated/site/types.gen.ts'
import { useFormat } from '../hooks/useFormat.ts'
import { StatCard } from './StatCard.tsx'

interface Labelled {
  label: string
  count: number
}

// The entries of a count by Skipped reason or Deferral cause that hold anything, labelled: the
// closed lists name every reason and cause (ADR 0178), most of them at zero, which show nothing.
function useLabelled(
  skipped: SkippedReasonCount[] | undefined,
  waiting: DeferralCauseCount[] | undefined,
): { skipped: Labelled[]; waiting: Labelled[] } {
  const { t } = useTranslation()
  return {
    skipped: (skipped ?? [])
      .filter(({ count }) => count > 0)
      .map(({ reason, count }) => ({ label: t(($) => $.outcomes.skippedReason[reason]), count })),
    waiting: (waiting ?? [])
      .filter(({ count }) => count > 0)
      .map(({ cause, count }) => ({ label: t(($) => $.outcomes.deferralCause[cause]), count })),
  }
}

// One block of OutcomeCards: its title, its hint and a card per entry; nothing while it has none.
function OutcomeBlock({
  title,
  hint,
  entries,
}: {
  title: string
  hint: string
  entries: Labelled[]
}) {
  if (entries.length === 0) {
    return null
  }
  return (
    <Stack gap="xs">
      <Title order={5}>{title}</Title>
      <Text size="sm" c="dimmed">
        {hint}
      </Text>
      <SimpleGrid cols={{ base: 2, sm: 3 }}>
        {entries.map(({ label, count }) => (
          <StatCard key={label} label={label} value={count} />
        ))}
      </SimpleGrid>
    </Stack>
  )
}

// Why a report's sends were Skipped and what its sends wait for now, as cards under a title each;
// a block with nothing in it is left out, and so is the whole while both are.
export function OutcomeCards({
  skippedByReason,
  waitingByCause,
}: {
  skippedByReason: SkippedReasonCount[]
  waitingByCause: DeferralCauseCount[]
}) {
  const { t } = useTranslation()
  const { skipped, waiting } = useLabelled(skippedByReason, waitingByCause)
  return (
    <>
      <OutcomeBlock
        title={t(($) => $.outcomes.skippedTitle)}
        hint={t(($) => $.outcomes.skippedHint)}
        entries={skipped}
      />
      <OutcomeBlock
        title={t(($) => $.outcomes.waitingTitle)}
        hint={t(($) => $.outcomes.waitingHint)}
        entries={waiting}
      />
    </>
  )
}

// The non-zero counts of a figure by Skipped reason or by Deferral cause, one dimmed line each,
// under the figure in a table cell; nothing while all are zero.
export function OutcomeLines({
  skippedByReason,
  waitingByCause,
}: {
  skippedByReason?: SkippedReasonCount[]
  waitingByCause?: DeferralCauseCount[]
}) {
  const { formatNumber } = useFormat()
  const { skipped, waiting } = useLabelled(skippedByReason, waitingByCause)
  return [...skipped, ...waiting].map(({ label, count }) => (
    <Text key={label} size="xs" c="dimmed">
      {label}: {formatNumber(count)}
    </Text>
  ))
}
