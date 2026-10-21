import { Alert, Button, Stack } from '@mantine/core'
import { startAuthentication } from '@simplewebauthn/browser'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteSecondFactorGetStatusQueryKey,
  sitePasskeysListOptions,
  sitePasskeysListQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { sitePasskeysBeginCheck, sitePasskeysFinishCheck } from '../generated/site/sdk.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { SigninProviders } from './SigninProviders.tsx'

// The browser's side of a fresh Passkey check (ADR 0052): the server's request options, naming
// the User's own Passkeys, go to the authenticator and its answer goes back. The server then
// reissues the session with the check, which counts as the proof for a few minutes.
async function checkPasskey() {
  const begun = await sitePasskeysBeginCheck({ throwOnError: true })
  const credential = await startAuthentication({ optionsJSON: begun.data.options })
  return sitePasskeysFinishCheck({ body: { credential: { ...credential } }, throwOnError: true })
}

// The proof a User with no password gives where others type the password: a fresh Passkey check
// when they hold a Passkey, or a fresh sign-in with a provider (ADR 0048, ADR 0052). `hint` says
// which step needs it. Either proof lands back on `returnTo` with the proof fresh.
export function IdentityProof({ hint, returnTo }: { hint: string; returnTo: string }) {
  const { t } = useTranslation()
  const passkeys = useQuery(sitePasskeysListOptions())
  const check = useResourceMutation({
    mutation: { mutationFn: checkPasskey },
    invalidate: [sitePasskeysListQueryKey(), siteSecondFactorGetStatusQueryKey()],
    errorTitle: t(($) => $.security.passkeyCheck.errorTitle),
  })
  const canCheck = passkeys.data?.available === true && passkeys.data.items.length > 0

  return (
    <Stack>
      <Alert color="blue" title={t(($) => $.security.proofTitle)}>
        {hint}
      </Alert>
      {canCheck ? (
        <Button onClick={() => check.mutate()} loading={check.isPending}>
          {t(($) => $.security.passkeyCheck.button)}
        </Button>
      ) : null}
      <SigninProviders returnTo={returnTo} separator={false} />
    </Stack>
  )
}
