import { Group, NumberInput } from '@mantine/core'
import type { NumberInputProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PER_DAY, PER_SECOND } from './providerLimit.ts'

interface ProviderLimitFieldsProps {
  perSecondProps: Partial<NumberInputProps>
  perDayProps: Partial<NumberInputProps>
  // An SES-compatible service needs both values (ADR 0067): blank is not offered as "no limit".
  required?: boolean
}

// The two Provider limit inputs (ADR 0023), shared by the connect form and the edit modal.
export function ProviderLimitFields({
  perSecondProps,
  perDayProps,
  required = false,
}: ProviderLimitFieldsProps) {
  const { t } = useTranslation()
  const hint = required ? undefined : t(($) => $.settings.integrations.providerLimit.hint)
  return (
    <Group grow align="flex-start">
      <NumberInput
        label={t(($) => $.settings.integrations.providerLimit.perSecond)}
        description={hint}
        required={required}
        allowDecimal={false}
        allowNegative={false}
        min={PER_SECOND.min}
        max={PER_SECOND.max}
        thousandSeparator=" "
        {...perSecondProps}
      />
      <NumberInput
        label={t(($) => $.settings.integrations.providerLimit.perDay)}
        description={hint}
        required={required}
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
