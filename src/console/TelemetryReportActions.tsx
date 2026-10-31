import { Alert, Anchor, Button, Code, Group, Loader, Modal, Stack, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { staffInstancePreviewTelemetryReportOptions } from '../generated/staff/@tanstack/react-query.gen.ts'

// TelemetryReportActions lets a Staff member read what the Instance sends: a link to the schema
// page (the Instance state names it, so it is right for every deployment) and a preview of the
// report as it would be sent right now, whether or not sending is on. The report is fetched only
// once the preview is opened.
export function TelemetryReportActions({ schemaUrl }: { schemaUrl: string }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const report = useQuery({
    ...staffInstancePreviewTelemetryReportOptions(),
    enabled: opened,
    retry: false,
  })

  return (
    <>
      <Group gap="md">
        <Anchor href={schemaUrl} target="_blank" rel="noopener noreferrer" size="sm">
          {t(($) => $.console.instance.report.schema)}
        </Anchor>
        <Button variant="subtle" size="compact-sm" onClick={open}>
          {t(($) => $.console.instance.report.preview)}
        </Button>
      </Group>
      <Modal
        opened={opened}
        onClose={close}
        size="lg"
        title={t(($) => $.console.instance.report.previewTitle)}
      >
        <Stack gap="sm">
          <Text size="sm" c="dimmed">
            {t(($) => $.console.instance.report.previewDescription)}
          </Text>
          {report.isPending ? <Loader size="sm" /> : null}
          {report.isError ? (
            <Alert color="red">{t(($) => $.console.instance.report.loadError)}</Alert>
          ) : null}
          {report.data ? <Code block>{JSON.stringify(report.data, null, 2)}</Code> : null}
        </Stack>
      </Modal>
    </>
  )
}
