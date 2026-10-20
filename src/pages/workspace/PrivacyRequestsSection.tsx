import { Alert, Box, Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteContactsEraseMutation,
  siteContactsLookupOptions,
  siteContactsLookupQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteContactLookup } from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'

// EraseAction is offered exactly while Erasure would find something: a Contact or
// delivery records hold the address (kept opt-outs alone do not count). A not-found
// answer means someone erased it meanwhile: say so and refresh the lookup.
function EraseAction({
  slug,
  email,
  held,
}: {
  slug: string
  email: string
  held: SiteContactLookup
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const confirm = useDeleteConfirmation()
  const apiErrorMessage = useApiErrorMessage()
  const refreshLookup = () =>
    queryClient.invalidateQueries({
      queryKey: siteContactsLookupQueryKey({ path: { slug }, query: { email } }),
    })
  const erase = useMutation({
    ...siteContactsEraseMutation(),
    onSuccess: async () => {
      notifications.show({
        color: 'teal',
        title: t(($) => $.notifications.successTitle),
        message: t(($) => $.settings.privacy.erased),
      })
      await refreshLookup()
    },
    onError: async (error) => {
      if (hasErrorStatus(error, [404])) {
        notifications.show({ message: t(($) => $.settings.privacy.nothingFound) })
        await refreshLookup()
        return
      }
      notifications.show({
        color: 'red',
        title: t(($) => $.alerts.eraseErrorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.notifications.errorMessage),
        ),
      })
    },
  })

  return (
    <Button
      color="red"
      variant="light"
      w="fit-content"
      disabled={!held.contactHeld && held.deliveryRecords === 0}
      loading={erase.isPending}
      onClick={() =>
        confirm({
          title: t(($) => $.settings.privacy.eraseConfirmTitle, { email }),
          description: t(($) => $.settings.privacy.eraseConfirmDescription),
          confirmLabel: t(($) => $.contacts.erase),
          onConfirm: () => erase.mutate({ path: { slug }, query: { email } }),
        })
      }
    >
      {t(($) => $.contacts.erase)}
    </Button>
  )
}

// LookupResult shows what the Workspace holds for the looked-up address, as counts.
// Erase and Export build on this result: they read the same lookup to decide whether
// they are offered (a Contact or delivery records hold the address).
function LookupResult({
  slug,
  email,
  held,
}: {
  slug: string
  email: string
  held: SiteContactLookup
}) {
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
      <EraseAction slug={slug} email={email} held={held} />
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
        {lookup.data ? <LookupResult slug={slug} email={email} held={lookup.data} /> : null}
      </Stack>
    </Box>
  )
}
