import { useTranslation } from 'react-i18next'

import { OneShotLinkPage } from '../../components/OneShotLinkPage.tsx'
import { siteAuthSigninNotMeMutationKey } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteAuthSigninNotMe } from '../../generated/site/sdk.gen.ts'
import { Route as signinNotMeRoute } from '../../routes/signin-not-me.tsx'

// Opening the link is the report of a sign-in that was not the User's.
export function SigninNotMePage() {
  const { t } = useTranslation()
  const { token } = signinNotMeRoute.useSearch()

  return (
    <OneShotLinkPage
      queryKey={siteAuthSigninNotMeMutationKey({ body: { token } })}
      send={async () => siteAuthSigninNotMe({ body: { token }, throwOnError: true })}
      copy={{
        working: t(($) => $.signinNotMe.working),
        successTitle: t(($) => $.signinNotMe.successTitle),
        successBody: t(($) => $.signinNotMe.successBody),
        goToLogin: t(($) => $.signinNotMe.goToLogin),
        errorTitle: t(($) => $.signinNotMe.errorTitle),
        errorBody: t(($) => $.signinNotMe.errorBody),
      }}
    />
  )
}
