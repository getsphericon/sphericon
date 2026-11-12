import { Button, Group, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { requiresProviderLimit, useProviderLimitValidators } from './providerLimit.ts'
import type { ProviderLimitValues } from './providerLimit.ts'
import { ProviderLimitFields } from './ProviderLimitFields.tsx'

// What a save of the limits sends: each a whole number, or null for no limit.
export interface ProviderLimitPayload {
  maxPerSecond: number | null
  maxPerDay: number | null
}

// The two Provider limit inputs. Whether both are required (an SES-compatible service) is the
// stored Integration's; the inputs are checked by the provider limit's own rules, in their words,
// before the save.
export function ProviderLimitForm({
  form,
  isPending,
  onSubmit,
  onCancel,
  stored,
}: ResourceFormProps<ProviderLimitPayload, ProviderLimitValues, SiteIntegrationResource>) {
  const { t } = useTranslation()
  const required =
    stored !== undefined &&
    requiresProviderLimit(
      stored.config.kind,
      stored.config.kind === 'ses' ? stored.config.endpoint : null,
    )
  const validate = useProviderLimitValidators(() => required)

  const submit = (event?: Parameters<ReturnType<typeof form.onSubmit>>[0]) => {
    const values = form.getValues()
    const errors = Object.fromEntries(
      (['maxPerSecond', 'maxPerDay'] as const).flatMap((field) => {
        const error = validate[field](values[field], values)
        return error ? [[field, error]] : []
      }),
    )
    if (Object.keys(errors).length > 0) {
      event?.preventDefault()
      form.setErrors(errors)
      return
    }
    form.onSubmit(onSubmit)(event)
  }

  return (
    <form onSubmit={submit}>
      <Stack>
        <Text c="dimmed" size="sm">
          {required
            ? t(($) => $.settings.integrations.providerLimit.descriptionRequired)
            : t(($) => $.settings.integrations.providerLimit.description)}
        </Text>
        <ProviderLimitFields
          perSecondProps={form.getInputProps('maxPerSecond')}
          perDayProps={form.getInputProps('maxPerDay')}
          required={required}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.settings.integrations.providerLimit.cancel)}
          </Button>
          <Button type="submit" loading={isPending}>
            {t(($) => $.settings.integrations.providerLimit.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
