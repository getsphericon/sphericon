import {
  Alert,
  Anchor,
  Button,
  Group,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import {
  WebAuthnAbortService,
  browserSupportsWebAuthn,
  browserSupportsWebAuthnAutofill,
  startAuthentication,
} from '@simplewebauthn/browser'
import { IconFingerprint } from '@tabler/icons-react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate, useRouter, useSearch } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AuthLayout } from '../../components/AuthLayout.tsx'
import { OneTimeCodeInput } from '../../components/OneTimeCodeInput.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { SigninProviders } from '../../components/SigninProviders.tsx'
import {
  siteAuthLoginMutation,
  siteAuthResendVerificationMutation,
  siteAuthSecondFactorMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { sitePasskeyLoginBegin, sitePasskeyLoginFinish } from '../../generated/site/sdk.gen.ts'
import type { SiteLoginInput, SiteLoginResult } from '../../generated/site/types.gen.ts'
import { zSiteSigninError } from '../../generated/site/zod.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as forgotPasswordRoute } from '../../routes/forgot-password.tsx'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as signUpRoute } from '../../routes/sign-up.tsx'
import { isEmailVerificationRequiredError } from '../../utils/apiErrors.ts'

// Where a Passkey login started: the person pressed the button, or the page offered the Passkeys
// in the email field's autofill list (the conditional UI) until `signal` says the offer is over.
type PasskeyAttempt = { from: 'button' } | { from: 'autofill'; signal: AbortSignal }

// The browser's side of a Passkey login (ADR 0052): the server's request options go to the
// authenticator, which offers the Passkeys it holds for this host, and its answer goes back. No
// email is typed: the server finds the User by the credential. From autofill the browser offers
// them in the email field and waits for the person to choose; an offer withdrawn while its options
// were on their way never reaches the browser, as there is no ceremony yet for the page to cancel.
async function loginWithPasskey(attempt: PasskeyAttempt) {
  const begun = await sitePasskeyLoginBegin({ throwOnError: true })
  if (attempt.from === 'autofill') {
    attempt.signal.throwIfAborted()
  }
  const credential = await startAuthentication({
    optionsJSON: begun.data.options,
    useBrowserAutofill: attempt.from === 'autofill',
  })
  const finished = await sitePasskeyLoginFinish({
    body: { credential: { ...credential } },
    throwOnError: true,
  })
  return finished.data
}

// The person dismissed the browser's prompt, or a newer ceremony replaced this one: not a failure
// worth a notification.
function isDeclined(error: unknown) {
  const names = new Set(['NotAllowedError', 'AbortError'])
  return (
    error instanceof Error &&
    (names.has(error.name) || (error.cause instanceof Error && names.has(error.cause.name)))
  )
}

export function LoginPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const navigate = useNavigate()
  const router = useRouter()
  // Set when a guard (e.g. the OAuth consent screen) sent the user here to sign in.
  const search = useSearch({ strict: false })
  const { redirect } = search
  // Set when the server's provider callback sent the person back here without a session.
  const signinError = zSiteSigninError.safeParse(search.error).data

  const apiErrorMessage = useApiErrorMessage()

  const form = useForm<SiteLoginInput>({
    initialValues: {
      email: '',
      password: '',
    },
  })

  // Set when the password was right (or the provider's callback came back) but the User has
  // a Second factor (ADR 0020): the session starts only after the code step.
  const [challenge, setChallenge] = useState<string | null>(search.challenge ?? null)
  const codeForm = useForm({ initialValues: { code: '' } })

  // Set when the login was refused because the address is not proven yet (ADR 0030):
  // the refusal offers to send the verification link again to this address.
  const [unverifiedEmail, setUnverifiedEmail] = useState<string | null>(null)

  const resendMutation = useResourceMutation({
    mutation: siteAuthResendVerificationMutation(),
    successMessage: t(($) => $.login.resendSuccess),
    errorTitle: t(($) => $.login.resendErrorTitle),
  })

  const showError = (error: unknown) =>
    notify({
      color: 'red',
      title: t(($) => $.login.errorTitle),
      message: apiErrorMessage(
        error,
        t(($) => $.login.errorMessage),
      ),
    })

  // A session is real however it started, so a late success of a superseded attempt still signs
  // in; when two attempts both succeed (a password and a Passkey chosen from autofill), only the
  // first one leaves the page. Once it is leaving, what any other attempt answers is dropped.
  const leaving = useRef(false)
  const signedIn = async () => {
    if (leaving.current) {
      return
    }
    leaving.current = true
    try {
      await (redirect ? router.history.push(redirect) : navigate({ to: indexRoute.to }))
    } catch (error) {
      // The page did not leave: the next sign-in may try again.
      leaving.current = false
      throw error
    }
  }

  const loginMutation = useMutation({
    ...siteAuthLoginMutation(),
    onSuccess: async (result: SiteLoginResult) => {
      if (leaving.current) {
        return
      }
      if (result.outcome === 'challenge' && result.challenge) {
        setChallenge(result.challenge)
        return
      }
      return signedIn()
    },
    onError: (error, variables) => {
      if (leaving.current) {
        return
      }
      // Drop the rejected password so the user retypes a fresh one.
      form.setFieldValue('password', '')
      if (isEmailVerificationRequiredError(error)) {
        setUnverifiedEmail(variables.body.email)
        return
      }
      setUnverifiedEmail(null)
      showError(error)
    },
  })

  // The code step is one attempt: Start over abandons it (reset), and what its code earns comes
  // back only to the step it was typed on, through the callbacks of that mutate call, which TanStack
  // drops once the mutation is reset. Its challenge cannot tell the steps apart: a login within the
  // same second is handed the same string. A late success still signs in (the session is real).
  const secondStepMutation = useMutation({
    ...siteAuthSecondFactorMutation(),
    onSuccess: signedIn,
  })
  const submitCode = (code: string, stepChallenge: string) =>
    secondStepMutation.mutate(
      { body: { challenge: stepChallenge, code } },
      {
        onError: (error) => {
          codeForm.setFieldValue('code', '')
          showError(error)
        },
      },
    )

  // The Passkey button, and the conditional UI that runs without a click: the browser offers the
  // person's Passkeys in the email field and the login finishes when one is chosen. Only the button
  // reports a failure; the conditional ceremony is silent. It belongs to the password view: it ends
  // when the page asks for the code or the person leaves, and starts again after Start over.
  const passkeyMutation = useMutation({
    mutationFn: loginWithPasskey,
    onSuccess: signedIn,
    onError: (error, attempt) => {
      if (attempt.from === 'button' && !isDeclined(error)) {
        showError(error)
      }
    },
  })
  const { mutate: passkeyLogin } = passkeyMutation
  const onCodeStep = challenge !== null
  useEffect(() => {
    const offer = new AbortController()
    const start = async () => {
      const supported = await browserSupportsWebAuthnAutofill()
      if (supported && !offer.signal.aborted) {
        passkeyLogin({ from: 'autofill', signal: offer.signal })
      }
    }
    if (!onCodeStep) {
      void start()
    }
    return () => {
      offer.abort()
      WebAuthnAbortService.cancelCeremony()
    }
  }, [passkeyLogin, onCodeStep])

  // One method at a time: a password login and the Passkey button wait for each other. The
  // conditional ceremony is pending for as long as the password view is open, so it blocks nothing.
  const passkeyButtonPending =
    passkeyMutation.isPending && passkeyMutation.variables.from === 'button'

  const startOver = () => {
    secondStepMutation.reset()
    setChallenge(null)
    codeForm.reset()
    form.setFieldValue('password', '')
  }

  if (challenge) {
    return (
      <AuthLayout>
        <Title order={3}>{t(($) => $.login.secondStepTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.login.secondStepDescription)}
        </Text>

        <form onSubmit={codeForm.onSubmit((values) => submitCode(values.code.trim(), challenge))}>
          <Stack>
            <OneTimeCodeInput
              label={t(($) => $.login.codeLabel)}
              recovery
              data-autofocus
              {...codeForm.getInputProps('code')}
            />
            <Group justify="space-between" align="center">
              <Anchor component="button" type="button" size="sm" onClick={startOver}>
                {t(($) => $.login.startOver)}
              </Anchor>
              <Button type="submit" loading={secondStepMutation.isPending}>
                {t(($) => $.login.verifyButton)}
              </Button>
            </Group>
          </Stack>
        </form>
      </AuthLayout>
    )
  }

  const handleSubmit = (values: SiteLoginInput) => {
    setUnverifiedEmail(null)
    loginMutation.mutate({
      body: {
        email: values.email.trim(),
        password: values.password,
      },
    })
  }

  return (
    <AuthLayout>
      <Title order={3}>{t(($) => $.login.title)}</Title>

      {signinError ? (
        <Alert color="red" title={t(($) => $.signin.errorTitle)}>
          {t(($) => $.signin.errors[signinError])}
        </Alert>
      ) : null}

      {unverifiedEmail ? (
        <Alert color="yellow" title={t(($) => $.login.verifyRequiredTitle)}>
          <Stack gap="xs" align="flex-start">
            <Text size="sm">{t(($) => $.login.verifyRequiredMessage)}</Text>
            <Button
              variant="light"
              size="xs"
              loading={resendMutation.isPending}
              onClick={() => resendMutation.mutate({ body: { email: unverifiedEmail } })}
            >
              {t(($) => $.login.resendVerification)}
            </Button>
          </Stack>
        </Alert>
      ) : null}

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <TextInput
            label={t(($) => $.login.emailLabel)}
            type="email"
            autoComplete="username webauthn"
            required
            {...form.getInputProps('email')}
          />
          <PasswordInput
            label={t(($) => $.login.passwordLabel)}
            required
            {...form.getInputProps('password')}
          />

          <Group justify="space-between" align="center">
            <Stack gap={2}>
              <AnchorLink to={signUpRoute.to} size="sm">
                {t(($) => $.login.signUpLink)}
              </AnchorLink>
              <AnchorLink to={forgotPasswordRoute.to} size="sm">
                {t(($) => $.login.forgotPasswordLink)}
              </AnchorLink>
            </Stack>
            <Button type="submit" loading={loginMutation.isPending} disabled={passkeyButtonPending}>
              {t(($) => $.login.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>

      <SigninProviders returnTo={redirect} separator="above" />

      {browserSupportsWebAuthn() ? (
        <Button
          variant="default"
          leftSection={<IconFingerprint size={18} />}
          loading={passkeyButtonPending}
          disabled={loginMutation.isPending}
          onClick={() => passkeyLogin({ from: 'button' })}
        >
          {t(($) => $.login.passkeyButton)}
        </Button>
      ) : null}
    </AuthLayout>
  )
}
