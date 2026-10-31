import { Card, Container, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { LegalMarkdown } from '../../components/LegalMarkdown.tsx'
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
  const { key } = legalDocumentRoute.useParams()
  const query = useQuery({ ...siteLegalDocumentsGetOptions({ path: { key } }), retry: false })

  return (
    <Container size="md" py="xl">
      {query.isPending && <PageSkeleton />}
      {query.isError &&
        (hasErrorStatus(query.error, [404]) ? (
          <Text c="dimmed">{t(($) => $.legal.notFound)}</Text>
        ) : (
          <ApiErrorAlert
            error={query.error}
            title={t(($) => $.legal.loadError)}
            fallback={t(($) => $.notifications.errorMessage)}
          />
        ))}
      {query.data && (
        <Stack>
          <Title order={1}>{query.data.title}</Title>
          <Text c="dimmed" size="sm">
            {t(($) => $.legal.version, {
              version: query.data.version,
              date: formatDate(query.data.publishedAt),
            })}
          </Text>
          <Card withBorder padding="xl">
            <LegalMarkdown markdown={query.data.markdown} />
          </Card>
        </Stack>
      )}
    </Container>
  )
}
