import { useTranslation } from 'react-i18next'

import type { SiteIntegrationProvider } from '../../generated/site/types.gen.ts'
import { zMaxPerDay, zMaxPerSecond } from '../../generated/site/zod.gen.ts'

// A limit field holds a number, or an empty string when the user left it blank
// (blank means "no limit").
export interface ProviderLimitValues {
  maxPerSecond: number | string
  maxPerDay: number | string
}

export const NO_LIMITS: ProviderLimitValues = { maxPerSecond: '', maxPerDay: '' }

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
export const PER_SECOND = {
  min: zMaxPerSecond.minValue ?? 1,
  max: zMaxPerSecond.maxValue ?? 1,
}
export const PER_DAY = {
  min: zMaxPerDay.minValue ?? 1,
  max: zMaxPerDay.maxValue ?? 1,
}

// The API field for a form value: null (no limit) for a blank input.
export function toLimit(value: number | string): number | null {
  return value === '' ? null : Number(value)
}

// Form values for a stored limit: a blank input when there is none.
export function fromLimit(limit: number | null): number | string {
  return limit ?? ''
}

// Whether an Integration must carry both values (ADR 0067): an SES one with its own endpoint is an
// SES-compatible service, which cannot report its limits. The server refuses it otherwise.
export function requiresProviderLimit(
  kind: SiteIntegrationProvider,
  endpoint: string | null | undefined,
): boolean {
  return kind === 'ses' && (endpoint ?? '').trim() !== ''
}

// Validators for useForm: a value must be a whole number in range, and blank (no limit) is fine
// unless `required` says the values in the form need the limit.
export function useProviderLimitValidators<Values>(
  required: (values: Values) => boolean = () => false,
) {
  const { t } = useTranslation()
  const check = (schema: typeof zMaxPerSecond, bounds: { min: number; max: number }) => {
    return (value: number | string, values: Values) => {
      if (value === '') {
        return required(values) ? t(($) => $.settings.integrations.providerLimit.required) : null
      }
      return schema.safeParse(value).success
        ? null
        : t(($) => $.settings.integrations.providerLimit.invalid, bounds)
    }
  }
  return {
    maxPerSecond: check(zMaxPerSecond, PER_SECOND),
    maxPerDay: check(zMaxPerDay, PER_DAY),
  }
}
