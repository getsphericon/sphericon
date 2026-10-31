import { useTranslation } from 'react-i18next'

import { siteSendingDomainsVerifyMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import { useNotify } from './useNotify.ts'
import { useResourceMutation } from './useResourceMutation.ts'

// useVerifySendingDomain runs the DNS check of a Sending domain and waits for it: the mutation is
// pending until the check is done, then a toast says whether the domain is verified.
export function useVerifySendingDomain() {
  const { t } = useTranslation()
  const notify = useNotify()
  return useResourceMutation({
    mutation: siteSendingDomainsVerifyMutation(),
    errorTitle: t(($) => $.settings.sendingDomains.verifyError),
    onDone: (domain) => {
      notify(
        domain.verified
          ? {
              color: 'teal',
              title: t(($) => $.notifications.successTitle),
              message: t(($) => $.settings.sendingDomains.verifiedNow),
            }
          : {
              color: 'yellow',
              title: t(($) => $.settings.sendingDomains.notVerifiedTitle),
              message: t(($) => $.settings.sendingDomains.notVerifiedYet),
            },
      )
    },
  })
}
