import { Button, Group, NumberInput, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { zMaxPerDay } from '../generated/staff/zod.gen.ts'
import { useSchemaForm } from '../resources/schemaForm.ts'

// The contract's bounds (TypeSpec -> zod) are the single source for the input's range.
const PER_DAY = { min: zMaxPerDay.minValue ?? 1, max: zMaxPerDay.maxValue ?? 1 }

// The form of a Send cap (ADR 0067), a Workspace's or the Instance's default: messages per rolling
// 24 hours, within the contract's bounds. The caller sends it.
export function SendCapForm({
  perDay,
  description,
  pending,
  onSubmit,
  onCancel,
}: {
  perDay: number | null
  description: string
  pending: boolean
  onSubmit: (body: { perDay: number }) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  // A blank NumberInput holds ''; the generated MaxPerDay judges the value, and one message names
  // its bounds whatever was wrong.
  const schema = z.object({
    perDay: z
      .union([z.number(), z.literal('')])
      .refine((value) => zMaxPerDay.safeParse(value).success, {
        error: t(($) => $.console.sendCapForm.invalid, PER_DAY),
      })
      .transform(Number),
  })
  const form = useSchemaForm(schema, { perDay: perDay ?? '' })

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <Text size="sm" c="dimmed">
          {description}
        </Text>
        <NumberInput
          label={t(($) => $.console.sendCapForm.perDayLabel)}
          withAsterisk
          data-autofocus
          allowDecimal={false}
          allowNegative={false}
          min={PER_DAY.min}
          max={PER_DAY.max}
          clampBehavior="none"
          {...form.getInputProps('perDay')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={pending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
