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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
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
import { Route as registerRoute } from '../../routes/register.tsx'
import { isEmailVerificationRequiredError } from '../../utils/apiErrors.ts'

// The browser's side of a Passkey login (ADR 0052): the server's request options go to the
// authenticator, which offers the Passkeys it holds for this host, and its answer goes back. No
// email is typed: the server finds the User by the credential. With `useBrowserAutofill` the
// browser offers them in the email field's autofill list and waits for the person to choose.
async function loginWithPasskey(useBrowserAutofill: boolean) {
  const begun = await sitePasskeyLoginBegin({ throwOnError: true })
  const credential = await startAuthentication({
    optionsJSON: begun.data.options,
    useBrowserAutofill,
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

  const signedIn = async () =>
    redirect ? router.history.push(redirect) : navigate({ to: indexRoute.to })

  const loginMutation = useMutation({
    ...siteAuthLoginMutation(),
    onSuccess: async (result: SiteLoginResult) => {
      if (result.outcome === 'challenge' && result.challenge) {
        setChallenge(result.challenge)
        return
      }
      return signedIn()
    },
    onError: (error, variables) => {
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

  const secondStepMutation = useMutation({
    ...siteAuthSecondFactorMutation(),
    onSuccess: signedIn,
    onError: (error) => {
      codeForm.setFieldValue('code', '')
      showError(error)
    },
  })

  // The Passkey button, and the conditional UI that runs without a click: the browser offers the
  // person's Passkeys in the email field and the login finishes when one is chosen. Only the button
  // reports a failure; the conditional ceremony is silent, and ends when the person leaves the page.
  const passkeyMutation = useMutation({
    mutationFn: loginWithPasskey,
    onSuccess: signedIn,
    onError: (error, useBrowserAutofill) => {
      if (!useBrowserAutofill && !isDeclined(error)) {
        showError(error)
      }
    },
  })
  const { mutate: passkeyLogin } = passkeyMutation
  useEffect(() => {
    let active = true
    const offer = async () => {
      const supported = await browserSupportsWebAuthnAutofill()
      if (supported && active) {
        passkeyLogin(true)
      }
    }
    void offer()
    return () => {
      active = false
      WebAuthnAbortService.cancelCeremony()
    }
  }, [passkeyLogin])

  const startOver = () => {
    setChallenge(null)
    codeForm.reset()
    form.setFieldValue('password', '')
  }

  if (challenge) {
    return (
      <CenteredLayout>
        <Title order={3}>{t(($) => $.login.secondStepTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.login.secondStepDescription)}
        </Text>

        <form
          onSubmit={codeForm.onSubmit((values) =>
            secondStepMutation.mutate({ body: { challenge, code: values.code.trim() } }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.login.codeLabel)}
              autoComplete="one-time-code"
              required
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
      </CenteredLayout>
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
    <CenteredLayout>
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
              <AnchorLink to={registerRoute.to} size="sm">
                {t(($) => $.login.registerLink)}
              </AnchorLink>
              <AnchorLink to={forgotPasswordRoute.to} size="sm">
                {t(($) => $.login.forgotPasswordLink)}
              </AnchorLink>
            </Stack>
            <Button type="submit" loading={loginMutation.isPending}>
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
          loading={passkeyMutation.isPending && !passkeyMutation.variables}
          onClick={() => passkeyLogin(false)}
        >
          {t(($) => $.login.passkeyButton)}
        </Button>
      ) : null}
    </CenteredLayout>
  )
}
