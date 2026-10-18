import { Group, NumberInput, type NumberInputProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { zSiteMaxPerDay, zSiteMaxPerSecond } from '../../generated/site/zod.gen.ts'

// A limit field holds a number, or an empty string when the operator left it blank
// (blank means "no limit").
export interface SendLimitValues {
  maxPerSecond: number | string
  maxPerDay: number | string
}

export const NO_LIMITS: SendLimitValues = { maxPerSecond: '', maxPerDay: '' }

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
const PER_SECOND = {
  min: zSiteMaxPerSecond.minValue ?? 1,
  max: zSiteMaxPerSecond.maxValue ?? 1,
}
const PER_DAY = {
  min: zSiteMaxPerDay.minValue ?? 1,
  max: zSiteMaxPerDay.maxValue ?? 1,
}

// The API field for a form value: null (no limit) for a blank input.
export function toLimit(value: number | string): number | null {
  return value === '' ? null : Number(value)
}

// Form values for a stored limit: a blank input when there is none.
export function fromLimit(limit: number | null): number | string {
  return limit ?? ''
}

// Validators for useForm: blank is fine, anything else must be a whole number in range.
export function useSendLimitValidators() {
  const { t } = useTranslation()
  const check = (schema: typeof zSiteMaxPerSecond, bounds: { min: number; max: number }) => {
    return (value: number | string) =>
      value === '' || schema.safeParse(value).success
        ? null
        : t(($) => $.settings.integrations.limits.invalid, bounds)
  }
  return {
    maxPerSecond: check(zSiteMaxPerSecond, PER_SECOND),
    maxPerDay: check(zSiteMaxPerDay, PER_DAY),
  }
}

interface SendLimitFieldsProps {
  perSecondProps: Partial<NumberInputProps>
  perDayProps: Partial<NumberInputProps>
}

// The two Send rate limit inputs (ADR 0023), shared by the connect form and the edit modal.
export function SendLimitFields({ perSecondProps, perDayProps }: SendLimitFieldsProps) {
  const { t } = useTranslation()
  return (
    <Group grow align="flex-start">
      <NumberInput
        label={t(($) => $.settings.integrations.limits.perSecond)}
        description={t(($) => $.settings.integrations.limits.hint)}
        allowDecimal={false}
        allowNegative={false}
        min={PER_SECOND.min}
        max={PER_SECOND.max}
        thousandSeparator=" "
        {...perSecondProps}
      />
      <NumberInput
        label={t(($) => $.settings.integrations.limits.perDay)}
        description={t(($) => $.settings.integrations.limits.hint)}
        allowDecimal={false}
        allowNegative={false}
        min={PER_DAY.min}
        max={PER_DAY.max}
        thousandSeparator=" "
        {...perDayProps}
      />
    </Group>
  )
}
