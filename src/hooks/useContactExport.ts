import { useMutation } from '@tanstack/react-query'
import { parse } from 'content-disposition-header'
import { saveAs } from 'file-saver'
import { useTranslation } from 'react-i18next'

import { siteContactsExport } from '../generated/site/sdk.gen.ts'
import type { SiteContactsExportData } from '../generated/site/types.gen.ts'
import { getApiErrorMessage } from '../utils/apiErrors.ts'
import { useNotify } from './useNotify.ts'

// The filename the server suggested in Content-Disposition (RFC 6266, `filename*` included).
function exportFilename(response: Response): string | undefined {
  const header = response.headers.get('Content-Disposition')
  return header ? parse(header).parameters.filename : undefined
}

// useContactExport downloads a Contact's ContactExportDocument (by id or by email) as a
// file. The generated mutation helper drops the response; the filename is the server's
// (Content-Disposition), so call the generated SDK function and keep both. The server
// streams the document as a download; it is read as the contract's typed
// ContactExportDocument and written out as the saved file.
export function useContactExport(workspaceId: string) {
  const { t } = useTranslation()
  const notify = useNotify()
  return useMutation({
    mutationFn: (query: NonNullable<SiteContactsExportData['query']>) =>
      siteContactsExport({ path: { workspaceId }, query, parseAs: 'json', throwOnError: true }),
    onSuccess: ({ data, response }) =>
      saveAs(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }),
        exportFilename(response),
      ),
    onError: (error) =>
      notify({
        color: 'red',
        title: t(($) => $.alerts.exportErrorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.notifications.errorMessage),
        ),
      }),
  })
}
