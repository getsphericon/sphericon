import { Card, Container, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { LegalMarkdown } from '../../components/LegalMarkdown.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import { siteLegalDocumentsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { Route as legalDocumentRoute } from '../../routes/legal.$key.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'

// The public page of a Sign-up item's Legal document (ADR 0062): the current version, read
// without signing in, its Markdown rendered.
export function LegalDocumentPage() {
  const { formatDate } = useDateFormat()
  const { t } = useTranslation()
  return (
    <Container size="md" py="xl">
      <QueryBoundary
        fallback={<PageSkeleton />}
        errorTitle={t(($) => $.legal.loadError)}
        renderError={(error) =>
          hasErrorStatus(error, [404]) ? (
            <Text c="dimmed">{t(($) => $.legal.notFound)}</Text>
          ) : undefined
        }
      >
        <LegalDocument />
      </QueryBoundary>
    </Container>
  )
}

function LegalDocument() {
  const { t } = useTranslation()
  const { key } = legalDocumentRoute.useParams()
  const legal = useSuspenseQuery({
    ...siteLegalDocumentsGetOptions({ path: { key } }),
    retry: false,
  }).data

  return (
    <Stack>
      <Title order={1}>{legal.title}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.legal.version, {
          version: legal.version,
          date: formatDate(legal.publishedAt),
        })}
      </Text>
      <Card withBorder padding="xl">
        <LegalMarkdown markdown={legal.markdown} />
      </Card>
    </Stack>
  )
}
