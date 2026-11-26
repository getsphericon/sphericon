import { useTranslation } from 'react-i18next'

import { OneShotLinkPage } from '../../components/OneShotLinkPage.tsx'
import { siteAuthConfirmEmailChangeMutationKey } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteAuthConfirmEmailChange } from '../../generated/site/sdk.gen.ts'
import { Route as confirmEmailChangeRoute } from '../../routes/confirm-email-change.tsx'

// Opening the link is the confirmation of the new address.
export function ConfirmEmailChangePage() {
  const { t } = useTranslation()
  const { token } = confirmEmailChangeRoute.useSearch()

  return (
    <OneShotLinkPage
      queryKey={siteAuthConfirmEmailChangeMutationKey({ body: { token } })}
      send={async () => siteAuthConfirmEmailChange({ body: { token }, throwOnError: true })}
      copy={{
        working: t(($) => $.confirmEmailChange.verifying),
        successTitle: t(($) => $.confirmEmailChange.successTitle),
        successBody: t(($) => $.confirmEmailChange.successBody),
        goToLogin: t(($) => $.confirmEmailChange.goToLogin),
        errorTitle: t(($) => $.confirmEmailChange.errorTitle),
        errorBody: t(($) => $.confirmEmailChange.errorBody),
      }}
    />
  )
}
