import { Button, Card, Group, Progress, Radio, Stack, Text, TextInput, Title } from '@mantine/core'
import { useCounter } from '@mantine/hooks'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { SubmitEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import {
  siteSignupSurveySubmitMutation,
  siteUserGetMeQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  SignupCompanySize,
  SignupGoal,
  SignupRole,
  SignupSource,
} from '../../generated/site/types.gen.ts'
import { zSiteSignupSurveyInput } from '../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { blankSurvey, OTHER, signupSurveySchema } from './signupSurveySchema.ts'
import type { Question } from './signupSurveySchema.ts'

interface Step {
  question: Question
  required: boolean
  title: string
  options: { value: string; label: string }[]
}

// The longest a question's "Other" text may be (the contract's limit).
function otherMaxLength(question: Question) {
  return zSiteSignupSurveyInput.shape[`${question}Other`].unwrap().maxLength ?? undefined
}

// The options of a question, one per value of its contract enum in the contract's order, each
// worded by labels: a value without its words fails to compile.
function choices<T extends string>(values: Record<string, T>, labels: Record<T, string>) {
  return Object.values(values).map((value) => ({ value, label: labels[value] }))
}

// The Sign-up survey: four screens, one question each, answers sent once after the last.
// Nothing is kept before that, so a closed tab starts the survey again at the next sign-in.
export function SignupSurveyPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const form = useSchemaForm(signupSurveySchema, blankSurvey)

  const steps: Step[] = [
    {
      question: 'role',
      required: true,
      title: t(($) => $.signupSurvey.role.title),
      options: choices(SignupRole, {
        founder: t(($) => $.signupSurvey.role.founder),
        marketer: t(($) => $.signupSurvey.role.marketer),
        developer: t(($) => $.signupSurvey.role.developer),
        product: t(($) => $.signupSurvey.role.product),
        other: t(($) => $.signupSurvey.other),
      }),
    },
    {
      question: 'companySize',
      required: true,
      title: t(($) => $.signupSurvey.companySize.title),
      options: choices(SignupCompanySize, {
        '1': t(($) => $.signupSurvey.companySize.size1),
        '2_10': t(($) => $.signupSurvey.companySize.size2to10),
        '11_50': t(($) => $.signupSurvey.companySize.size11to50),
        '51_200': t(($) => $.signupSurvey.companySize.size51to200),
        '201_1000': t(($) => $.signupSurvey.companySize.size201to1000),
        '1000_plus': t(($) => $.signupSurvey.companySize.size1000plus),
        other: t(($) => $.signupSurvey.other),
      }),
    },
    {
      question: 'mainGoal',
      required: true,
      title: t(($) => $.signupSurvey.mainGoal.title),
      options: choices(SignupGoal, {
        broadcasts: t(($) => $.signupSurvey.mainGoal.broadcasts),
        automations: t(($) => $.signupSurvey.mainGoal.automations),
        tracking: t(($) => $.signupSurvey.mainGoal.tracking),
        migrating: t(($) => $.signupSurvey.mainGoal.migrating),
        other: t(($) => $.signupSurvey.other),
      }),
    },
    {
      question: 'heardFrom',
      required: false,
      title: t(($) => $.signupSurvey.heardFrom.title),
      options: choices(SignupSource, {
        search: t(($) => $.signupSurvey.heardFrom.search),
        github: t(($) => $.signupSurvey.heardFrom.github),
        social: t(($) => $.signupSurvey.heardFrom.social),
        recommendation: t(($) => $.signupSurvey.heardFrom.recommendation),
        other: t(($) => $.signupSurvey.other),
      }),
    },
  ]

  const submit = useResourceMutation({
    mutation: siteSignupSurveySubmitMutation(),
    // The route guards read the current user from the cache: refresh it before leaving.
    errorTitle: t(($) => $.signupSurvey.errorTitle),
    // A 409 says the survey is not due (answered elsewhere, or the Instance stopped asking):
    // read the current user again and go where the guard sends them, with no error toast.
    handleError: async (error) => {
      if (!hasErrorStatus(error, [409])) {
        return false
      }
      await queryClient.invalidateQueries({ queryKey: siteUserGetMeQueryKey() })
      await navigate({ to: indexRoute.to })
      return true
    },
    onDone: async () => {
      await navigate({ to: indexRoute.to })
    },
  })

  const [index, { increment, decrement }] = useCounter(0, { min: 0, max: steps.length - 1 })
  const step = steps[index]
  if (!step) {
    return null
  }
  const isLast = index === steps.length - 1
  const otherField = `${step.question}Other` as const

  // Next checks the screen's question before the following one shows; Finish checks them all and
  // sends the answers.
  const next = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (form.validateFields([step.question, otherField])) {
      increment()
    }
  }

  return (
    <CenteredLayout maw={520}>
      <Card withBorder padding="xl">
        <form onSubmit={isLast ? form.onSubmit((body) => submit.mutate({ body })) : next}>
          <Stack>
            <Progress value={((index + 1) / steps.length) * 100} aria-hidden />
            <Text size="sm" c="dimmed">
              {t(($) => $.signupSurvey.progress, { current: index + 1, total: steps.length })}
            </Text>
            <Title order={3}>{step.title}</Title>
            <Radio.Group
              key={step.question}
              aria-label={step.title}
              {...form.getInputProps(step.question)}
            >
              <Stack gap="xs">
                {step.options.map((option) => (
                  <Radio key={option.value} value={option.value} label={option.label} />
                ))}
              </Stack>
            </Radio.Group>
            {form.values[step.question] === OTHER && (
              <TextInput
                label={t(($) => $.signupSurvey.otherLabel)}
                maxLength={otherMaxLength(step.question)}
                {...form.getInputProps(otherField)}
              />
            )}
            {!step.required && (
              <Text size="sm" c="dimmed">
                {t(($) => $.signupSurvey.optional)}
              </Text>
            )}
            <Group justify="space-between">
              <Button
                variant="default"
                disabled={index === 0 || submit.isPending}
                onClick={decrement}
              >
                {t(($) => $.signupSurvey.back)}
              </Button>
              <Button type="submit" loading={submit.isPending}>
                {isLast ? t(($) => $.signupSurvey.finish) : t(($) => $.signupSurvey.next)}
              </Button>
            </Group>
          </Stack>
        </form>
      </Card>
    </CenteredLayout>
  )
}
