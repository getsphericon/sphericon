import { Button, Group, Select, Stack, TagsInput, TextInput } from '@mantine/core'
import { DateInput } from '@mantine/dates'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { CountryFlag } from '../../components/CountryFlag.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { siteTagsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCreateContactInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { countryOptions } from '../../utils/countries.ts'
import { tagsCodec } from './tags.ts'

const timeZones = Intl.supportedValuesOf('timeZone')

// What the form submits: the create input, with the tags always given.
export type ContactPayload = Omit<SiteCreateContactInput, 'tags'> & { tags: string[] }

// Identity is multi-key: subject_id / email / phone are alias keys, any of which may be
// absent (ADR 0002); blank inputs are normalized away by the contact form schema. The subject
// id comes from the tracker and the API, so it is not typed here.
export function ContactForm({
  form,
  isPending,
  onSubmit,
  onCancel,
}: ResourceFormProps<ContactPayload>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  // A stored zone the browser's list lacks (UTC, an alias) stays selectable.
  const timeZone = form.values.timeZone ?? ''
  const zones =
    timeZone === '' || timeZones.includes(timeZone) ? timeZones : [...timeZones, timeZone]
  const tagsQuery = useQuery(siteTagsListOptions({ path: { workspaceId } }))
  const knownTags = useMemo(
    () => tagsQuery.data?.items.map((tag) => tag.name) ?? [],
    [tagsQuery.data],
  )
  // The form holds the tags as JSON text (see the contact resource).
  const tags = useMemo(() => {
    const parsed = tagsCodec.safeParse(form.values.tags)
    return parsed.success ? parsed.data : []
  }, [form.values.tags])

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput label={t(($) => $.table.email)} {...form.getInputProps('email')} />
        <TextInput label={t(($) => $.table.phone)} {...form.getInputProps('phone')} />
        <TextInput label={t(($) => $.table.firstName)} {...form.getInputProps('firstName')} />
        <TextInput label={t(($) => $.table.lastName)} {...form.getInputProps('lastName')} />
        <Select
          label={t(($) => $.table.timeZone)}
          data={zones}
          value={timeZone || null}
          onChange={(next) => form.setFieldValue('timeZone', next ?? '')}
          error={form.errors.timeZone}
          searchable
          clearable
        />
        <Select
          label={t(($) => $.table.country)}
          data={countryOptions}
          value={form.values.country || null}
          onChange={(next) => form.setFieldValue('country', next ?? '')}
          error={form.errors.country}
          leftSection={form.values.country ? <CountryFlag code={form.values.country} /> : null}
          searchable
          clearable
        />
        <TextInput label={t(($) => $.table.city)} {...form.getInputProps('city')} />
        <TextInput
          label={t(($) => $.table.locale)}
          description={t(($) => $.table.localeHint)}
          {...form.getInputProps('locale')}
        />
        <DateInput
          label={t(($) => $.table.birthday)}
          value={form.values.birthday || null}
          onChange={(next) => form.setFieldValue('birthday', next ?? '')}
          error={form.errors.birthday}
          valueFormat="LL"
          clearable
        />
        <TextInput label={t(($) => $.table.avatar)} {...form.getInputProps('avatar')} />
        <TagsInput
          label={t(($) => $.table.tags)}
          data={knownTags}
          value={tags}
          onChange={(next) => form.setFieldValue('tags', z.encode(tagsCodec, next))}
        />

        <Group justify="flex-end">
          {onCancel ? (
            <Button variant="default" onClick={onCancel}>
              {t(($) => $.actions.cancel)}
            </Button>
          ) : (
            <ButtonLink variant="default" to={contactsRoute.to} params={{ workspaceId }}>
              {t(($) => $.actions.cancel)}
            </ButtonLink>
          )}
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
