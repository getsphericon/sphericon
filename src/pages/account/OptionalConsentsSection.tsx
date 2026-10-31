import { Checkbox, Divider, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../components/RouterLink.tsx'
import {
  siteOptionalConsentsListOptions,
  siteOptionalConsentsSetMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteOptionalConsentDecision } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as legalDocumentRoute } from '../../routes/legal.$key.tsx'

// OptionalConsentsSection lets the User give or withdraw each optional consent in effect (ADR
// 0062), as easily as they agreed at sign-up. A box is ticked only while the consent is given; a
// User who never answered (they signed up before the item existed) sees it unticked and is asked
// nothing. With no optional consent in effect the section is not shown.
export function OptionalConsentsSection() {
  const { t } = useTranslation()
  const consentsQuery = useQuery(siteOptionalConsentsListOptions())

  const setConsent = useResourceMutation({
    mutation: siteOptionalConsentsSetMutation(),
    successMessage: t(($) => $.profile.consentsSaved),
    errorTitle: t(($) => $.profile.consentsErrorTitle),
  })

  const consents = consentsQuery.data?.items ?? []
  if (consents.length === 0) {
    return null
  }

  return (
    <Stack maw={420}>
      <Divider label={t(($) => $.profile.consentsSectionTitle)} />
      <Text size="sm" c="dimmed">
        {t(($) => $.profile.consentsDescription)}
      </Text>
      {consents.map((consent) => (
        <Checkbox
          key={consent.key}
          label={consent.title}
          checked={consent.decision === SiteOptionalConsentDecision.ACCEPTED}
          disabled={setConsent.isPending}
          onChange={(event) =>
            setConsent.mutate({
              path: { key: consent.key },
              body: { version: consent.version, accepted: event.currentTarget.checked },
            })
          }
          description={
            <AnchorLink
              to={legalDocumentRoute.to}
              params={{ key: consent.key }}
              target="_blank"
              size="xs"
            >
              {t(($) => $.signUp.readItem, { title: consent.title })}
            </AnchorLink>
          }
        />
      ))}
    </Stack>
  )
}
