import { Button, Group, NumberInput, Select, Stack, Textarea, TextInput } from '@mantine/core'
import { use } from 'react'
import { useTranslation } from 'react-i18next'

import { zAttributionWindowDays, zRetentionDays } from '../../../generated/site/zod.gen.ts'
import type { ResourceFormProps } from '../../../resources/defineResource.tsx'
import { boundsOf } from '../../../utils/bounds.ts'
import { TIME_ZONES } from '../../../utils/datetime.ts'
import { CanEdit } from './workspaceSettingsForm.ts'
import type {
  GeneralPayload,
  GeneralValues,
  RetentionPayload,
  RetentionValues,
  RevenuePayload,
  RevenueValues,
  TimezonePayload,
  TimezoneValues,
} from './workspaceSettingsForm.ts'

// The bounds the contract declares, read off its generated schemas.
const WINDOW_DAYS = boundsOf(zAttributionWindowDays)
const RETENTION_DAYS = boundsOf(zRetentionDays)

const CURRENCIES = Intl.supportedValuesOf('currency')

// The Save of a settings section, offered only to who may change it: anyone else sees the values
// read-only (the API refuses them).
function SaveButton({ isPending }: { isPending: boolean }) {
  const { t } = useTranslation()
  return use(CanEdit) ? (
    <Group justify="flex-end">
      <Button type="submit" loading={isPending}>
        {t(($) => $.actions.save)}
      </Button>
    </Group>
  ) : null
}

export function GeneralFields({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<GeneralPayload, GeneralValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack maw={420}>
        <TextInput
          label={t(($) => $.settings.nameLabel)}
          required
          {...form.getInputProps('name')}
        />
        <Textarea
          label={t(($) => $.settings.postalAddressLabel)}
          description={t(($) => $.settings.postalAddressDescription)}
          placeholder={t(($) => $.settings.postalAddressPlaceholder)}
          autosize
          minRows={2}
          {...form.getInputProps('postalAddress')}
        />
        <SaveButton isPending={isPending} />
      </Stack>
    </form>
  )
}

export function TimezoneFields({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<TimezonePayload, TimezoneValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack maw={420}>
        <Select
          label={t(($) => $.settings.timezoneLabel)}
          description={t(($) => $.settings.timezoneDescription)}
          data={TIME_ZONES}
          searchable
          allowDeselect={false}
          {...form.getInputProps('timezone')}
        />
        <SaveButton isPending={isPending} />
      </Stack>
    </form>
  )
}

export function RevenueFields({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<RevenuePayload, RevenueValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack maw={420}>
        <Select
          label={t(($) => $.settings.revenue.currencyLabel)}
          description={t(($) => $.settings.revenue.currencyDescription)}
          placeholder={t(($) => $.settings.revenue.currencyPlaceholder)}
          data={CURRENCIES}
          searchable
          allowDeselect={false}
          {...form.getInputProps('reportingCurrency')}
        />
        <NumberInput
          label={t(($) => $.settings.revenue.windowLabel)}
          description={t(($) => $.settings.revenue.windowDescription)}
          min={WINDOW_DAYS.min}
          max={WINDOW_DAYS.max}
          allowDecimal={false}
          allowNegative={false}
          clampBehavior="strict"
          {...form.getInputProps('attributionWindowDays')}
        />
        <SaveButton isPending={isPending} />
      </Stack>
    </form>
  )
}

export function RetentionFields({
  form,
  isPending,
  onSubmit,
  onCancel,
}: ResourceFormProps<RetentionPayload, RetentionValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <NumberInput
          data-autofocus
          label={t(($) => $.settings.auditLog.retentionLabel)}
          description={t(($) => $.settings.auditLog.retentionDescription)}
          placeholder={t(($) => $.settings.auditLog.retentionPlaceholder)}
          min={RETENTION_DAYS.min}
          max={RETENTION_DAYS.max}
          allowDecimal={false}
          allowNegative={false}
          clampBehavior="none"
          {...form.getInputProps('retentionDays')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
