import { NumberInput, Select } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useTranslation } from 'react-i18next'

import type { EntryPolicy } from '../../generated/site/types.gen.ts'
import { zEntryCooldown, zEntryPolicy } from '../../generated/site/zod.gen.ts'
import { boundsOf } from '../../utils/bounds.ts'
import { cooldownHours, cooldownSeconds, entryPolicyOf } from './definition.ts'

const SECONDS_PER_HOUR = 3600

// The fewest and most whole hours the contract's cooldown in seconds holds.
const { min: minSeconds, max: maxSeconds } = boundsOf(zEntryCooldown)
const MIN_HOURS = Math.ceil(minSeconds / SECONDS_PER_HOUR)
const MAX_HOURS = Math.floor(maxSeconds / SECONDS_PER_HOUR)

// The Entry policy of the Event Trigger (ADR 0127): whether a contact who enrolled before enrolls
// again once that enrollment ended.
export function EntryPolicyControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const labels: Record<EntryPolicy, string> = {
    once: t(($) => $.automations.entryPolicy.once),
    again: t(($) => $.automations.entryPolicy.again),
  }

  return (
    <Select
      label={label}
      description={t(($) => $.automations.entryPolicyHint)}
      allowDeselect={false}
      data={zEntryPolicy.options.map((value) => ({ value, label: labels[value] }))}
      value={entryPolicyOf(data)}
      onChange={(value) => props.handleChange(path, entryPolicyOf(value))}
    />
  )
}

// The cooldown under `again`, in whole hours: the node holds text of seconds, the contract's unit.
// The Trigger's form shows it only under `again`, the one policy it applies to.
export function CooldownControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()

  return (
    <NumberInput
      label={label}
      description={t(($) => $.automations.cooldownHint)}
      min={MIN_HOURS}
      max={MAX_HOURS}
      allowDecimal={false}
      allowNegative={false}
      value={cooldownHours(data)}
      onChange={(value) => props.handleChange(path, cooldownSeconds(value))}
    />
  )
}
