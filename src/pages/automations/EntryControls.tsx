import { Autocomplete, NumberInput, Select } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteEventsPropertyKeysOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { EntryPolicy } from '../../generated/site/types.gen.ts'
import { zEntryCooldown, zEntryPolicy } from '../../generated/site/zod.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { boundsOf } from '../../utils/bounds.ts'
import { cooldownHours, cooldownSeconds, entryPolicyOf } from './definition.ts'

const SECONDS_PER_HOUR = 3600

// The fewest and most whole hours the contract's cooldown in seconds holds.
const { min: minSeconds, max: maxSeconds } = boundsOf(zEntryCooldown)
const MIN_HOURS = Math.ceil(minSeconds / SECONDS_PER_HOUR)
const MAX_HOURS = Math.floor(maxSeconds / SECONDS_PER_HOUR)

// The Entry policy of the Event Trigger (ADR 0127): whether a contact who enrolled before enrolls
// again once that enrollment ended.
export function EntryPolicyControl({
  value,
  onChange,
}: {
  value: EntryPolicy
  onChange: (policy: EntryPolicy) => void
}) {
  const { t } = useTranslation()
  const labels: Record<EntryPolicy, string> = {
    once: t(($) => $.automations.entryPolicy.once),
    again: t(($) => $.automations.entryPolicy.again),
  }

  return (
    <Select
      label={t(($) => $.automations.entryPolicyLabel)}
      description={t(($) => $.automations.entryPolicyHint)}
      allowDeselect={false}
      data={zEntryPolicy.options.map((policy) => ({ value: policy, label: labels[policy] }))}
      value={value}
      onChange={(policy) => onChange(entryPolicyOf(policy))}
    />
  )
}

// The cooldown under `again`, in whole hours: the flow holds seconds, the contract's unit. The
// Trigger's panel shows it only under `again`, the one policy it applies to.
export function CooldownControl({
  value,
  onChange,
}: {
  value: number | null
  onChange: (seconds: number | null) => void
}) {
  const { t } = useTranslation()

  return (
    <NumberInput
      label={t(($) => $.automations.cooldownLabel)}
      description={t(($) => $.automations.cooldownHint)}
      min={MIN_HOURS}
      max={MAX_HOURS}
      allowDecimal={false}
      allowNegative={false}
      value={cooldownHours(value)}
      onChange={(hours) => onChange(cooldownSeconds(hours))}
    />
  )
}

// The Enrollment key of the Event Trigger (ADR 0165): a property of the triggering Event, such as
// order_id, by whose value a contact runs one enrollment per value. Property keys have no
// catalogue, so the field takes free text and suggests the keys seen on recent events of the
// Trigger's action, a hint and not a check.
export function EnrollmentKeyControl({
  action,
  value,
  onChange,
}: {
  action: string
  value: string
  onChange: (key: string) => void
}) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const trimmed = action.trim()
  const keys = useQuery({
    ...siteEventsPropertyKeysOptions({ path: { workspaceId }, query: { action: trimmed } }),
    enabled: trimmed !== '',
  })

  return (
    <Autocomplete
      label={t(($) => $.automations.enrollmentKeyLabel)}
      description={t(($) => $.automations.enrollmentKeyHint)}
      data={keys.data?.keys ?? []}
      value={value}
      onChange={onChange}
    />
  )
}
