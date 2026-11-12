import { Button, Select, Stack, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { zSiteCustomFieldType } from '../../generated/site/zod.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'

// What a Custom field's edit sends, and what its form holds: its name and its type. Its key never
// changes.
export interface CustomFieldPayload {
  name: string
  type: (typeof zSiteCustomFieldType.options)[number]
}
export interface CustomFieldValues {
  name: string
  type: string
}

export function CustomFieldForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CustomFieldPayload, CustomFieldValues>) {
  const { t } = useTranslation()
  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.customFields.nameLabel)}
          withAsterisk
          data-autofocus
          {...form.getInputProps('name')}
        />
        <Select
          label={t(($) => $.customFields.typeLabel)}
          description={t(($) => $.customFields.typeHint)}
          allowDeselect={false}
          data={zSiteCustomFieldType.options.map((type) => ({
            value: type,
            label: t(($) => $.customFields.types[type]),
          }))}
          {...form.getInputProps('type')}
        />
        <Button type="submit" loading={isPending}>
          {t(($) => $.actions.save)}
        </Button>
      </Stack>
    </form>
  )
}
