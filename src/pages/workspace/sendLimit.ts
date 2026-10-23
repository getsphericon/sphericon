import { useTranslation } from 'react-i18next'

import { zSiteMaxPerDay, zSiteMaxPerSecond } from '../../generated/site/zod.gen.ts'

// A limit field holds a number, or an empty string when the user left it blank
// (blank means "no limit").
export interface SendLimitValues {
  maxPerSecond: number | string
  maxPerDay: number | string
}

export const NO_LIMITS: SendLimitValues = { maxPerSecond: '', maxPerDay: '' }

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
export const PER_SECOND = {
  min: zSiteMaxPerSecond.minValue ?? 1,
  max: zSiteMaxPerSecond.maxValue ?? 1,
}
export const PER_DAY = {
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
