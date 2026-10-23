import { Group, NumberInput } from '@mantine/core'
import type { NumberInputProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PER_DAY, PER_SECOND } from './sendLimit.ts'

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
