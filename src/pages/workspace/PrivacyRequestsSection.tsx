import { Alert, Box, Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { siteContactsLookupOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteContactLookup } from '../../generated/site/types.gen.ts'

// LookupResult shows what the Workspace holds for the looked-up address, as counts.
// Erase and Export build on this result: they read the same lookup to decide whether
// they are offered (a Contact or delivery records hold the address).
function LookupResult({ held }: { held: SiteContactLookup }) {
  const { t } = useTranslation()
  const nothingHeld =
    !held.contactHeld &&
    held.events === 0 &&
    held.deliveryRecords === 0 &&
    held.unsubscribes === 0 &&
    held.suppressions === 0

  if (nothingHeld) {
    return <Text>{t(($) => $.settings.privacy.nothingHeld)}</Text>
  }
  return (
    <Stack gap={4}>
      <Title order={5}>{t(($) => $.settings.privacy.resultTitle)}</Title>
      <Text>
        {held.contactHeld
          ? t(($) => $.settings.privacy.contactHeld)
          : t(($) => $.settings.privacy.contactNotHeld)}
      </Text>
      <Text>{t(($) => $.settings.privacy.events, { n: held.events })}</Text>
      <Text>{t(($) => $.settings.privacy.deliveryRecords, { n: held.deliveryRecords })}</Text>
      <Text>{t(($) => $.settings.privacy.unsubscribes, { n: held.unsubscribes })}</Text>
      <Text>{t(($) => $.settings.privacy.suppressions, { n: held.suppressions })}</Text>
    </Stack>
  )
}

// PrivacyRequestsSection answers the first step of a Privacy request: type the address
// the person wrote from and see what the Workspace holds for it. The caller shows it to
// owners and admins only.
export function PrivacyRequestsSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const [email, setEmail] = useState('')
  const lookup = useQuery({
    ...siteContactsLookupOptions({ path: { slug }, query: { email } }),
    enabled: email !== '',
    retry: false,
  })

  return (
    <Box>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.privacy.description)}
      </Text>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          setEmail(draft.trim().toLowerCase())
        }}
      >
        <Group align="flex-end">
          <TextInput
            label={t(($) => $.settings.privacy.emailLabel)}
            value={draft}
            onChange={(event) => setDraft(event.currentTarget.value)}
            miw={280}
          />
          <Button type="submit" loading={lookup.isFetching}>
            {t(($) => $.settings.privacy.lookup)}
          </Button>
        </Group>
      </form>
      <Stack mt="md">
        {lookup.isError ? (
          <Alert color="red">{t(($) => $.settings.privacy.lookupError)}</Alert>
        ) : null}
        {lookup.data ? <LookupResult held={lookup.data} /> : null}
      </Stack>
    </Box>
  )
}
