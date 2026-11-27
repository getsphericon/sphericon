import { Button, FileInput, Stack, Text, Title } from '@mantine/core'
import { IconFileTypeCsv } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteContactImportsUploadMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as importRoute } from '../../routes/workspaces/$workspaceId/contacts/import.tsx'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'

// The request's check: the contract's binary body is a string to the generated zod schema, so the
// upload checks it is a file instead; the server reads and refuses the file itself.
const fileRequest = z.looseObject({ body: z.instanceof(Blob) })
const acceptFile = async (data: unknown) => fileRequest.parseAsync(data)

// The first step of a CSV import (ADR 0138): the Member picks a file, which is uploaded whole and
// read by the server; a file it refuses says why.
export function ContactImportUploadPage() {
  const { t } = useTranslation()
  const { workspaceId } = importRoute.useParams()
  const navigate = useNavigate()
  const [file, setFile] = useState<File | null>(null)
  const upload = useMutation({
    ...siteContactImportsUploadMutation(),
    onSuccess: (imp) =>
      void navigate({ to: importPageRoute.to, params: { workspaceId, importId: imp.id } }),
  })

  return (
    <Stack maw={640}>
      <Title order={2}>{t(($) => $.contactImports.uploadTitle)}</Title>
      <Text c="dimmed">{t(($) => $.contactImports.uploadDescription)}</Text>
      <FileInput
        label={t(($) => $.contactImports.file)}
        description={t(($) => $.contactImports.fileHint)}
        accept=".csv,text/csv"
        leftSection={<IconFileTypeCsv size={16} />}
        value={file}
        onChange={setFile}
        clearable
      />
      {upload.isError ? (
        <ApiErrorAlert error={upload.error} title={t(($) => $.contactImports.uploadError)} />
      ) : null}
      <Button
        w="fit-content"
        disabled={file === null}
        loading={upload.isPending}
        onClick={() => {
          if (file !== null) {
            upload.mutate({ path: { workspaceId }, body: file, requestValidator: acceptFile })
          }
        }}
      >
        {t(($) => $.contactImports.upload)}
      </Button>
    </Stack>
  )
}
