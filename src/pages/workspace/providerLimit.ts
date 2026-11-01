import { useTranslation } from 'react-i18next'

import type {
  SiteIntegrationProvider,
  SiteProviderLimitWarning,
} from '../../generated/site/types.gen.ts'
import { zMaxPerDay, zMaxPerSecond } from '../../generated/site/zod.gen.ts'
import { boundsOf } from '../../utils/bounds.ts'

// A limit field holds a number, or an empty string when the user left it blank
// (blank means "no limit").
export interface ProviderLimitValues {
  maxPerSecond: number | string
  maxPerDay: number | string
}

export const NO_LIMITS: ProviderLimitValues = { maxPerSecond: '', maxPerDay: '' }

// The contract's bounds (TypeSpec -> zod) are the single source for the inputs' range.
export const PER_SECOND = boundsOf(zMaxPerSecond)
export const PER_DAY = boundsOf(zMaxPerDay)

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

export interface ProviderLimitWarningCopy {
  warning: SiteProviderLimitWarning
  color: 'red' | 'yellow'
  // The short label on an Integration's row.
  badge: string
  // The alert above the list, for every Integration with the warning at once.
  title: (names: string[]) => string
  body: (count: number) => string
}

// What each Provider limit warning says, most serious first: a stopped account holds its sends
// (red, ADR 0067), a sandbox keeps mail from the contacts, probation may lead to a stop, no ceiling
// may get sends throttled, and an unread quota changes nothing about sending.
export function useProviderLimitWarnings(): ProviderLimitWarningCopy[] {
  const { t } = useTranslation()
  return [
    {
      warning: 'providerStopped',
      color: 'red',
      badge: t(($) => $.settings.integrations.providerLimit.badge.stopped),
      title: (names) =>
        t(($) => $.settings.integrations.providerLimit.stoppedTitle, {
          names,
          count: names.length,
        }),
      body: (count) => t(($) => $.settings.integrations.providerLimit.stoppedWarning, { count }),
    },
    {
      warning: 'providerSandbox',
      color: 'yellow',
      badge: t(($) => $.settings.integrations.providerLimit.badge.sandbox),
      title: (names) =>
        t(($) => $.settings.integrations.providerLimit.sandboxTitle, {
          names,
          count: names.length,
        }),
      body: () => t(($) => $.settings.integrations.providerLimit.sandboxWarning),
    },
    {
      warning: 'providerProbation',
      color: 'yellow',
      badge: t(($) => $.settings.integrations.providerLimit.badge.probation),
      title: (names) =>
        t(($) => $.settings.integrations.providerLimit.probationTitle, {
          names,
          count: names.length,
        }),
      body: (count) => t(($) => $.settings.integrations.providerLimit.probationWarning, { count }),
    },
    {
      warning: 'unlimited',
      color: 'yellow',
      badge: t(($) => $.settings.integrations.providerLimit.badge.unlimited),
      title: (names) =>
        t(($) => $.settings.integrations.providerLimit.unlimitedTitle, {
          names,
          count: names.length,
        }),
      body: () => t(($) => $.settings.integrations.providerLimit.unlimitedWarning),
    },
    {
      warning: 'providerQuotaUnavailable',
      color: 'yellow',
      badge: t(($) => $.settings.integrations.providerLimit.badge.quotaUnavailable),
      title: (names) =>
        t(($) => $.settings.integrations.providerLimit.quotaUnavailableTitle, {
          names,
          count: names.length,
        }),
      body: () => t(($) => $.settings.integrations.providerLimit.quotaUnavailableWarning),
    },
  ]
}
