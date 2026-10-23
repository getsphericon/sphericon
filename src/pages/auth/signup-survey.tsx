import { Button, Card, Group, Progress, Radio, Stack, Text, TextInput, Title } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteSignupSurveySubmitMutation,
  siteUserGetMeQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSignupSurveyInput } from '../../generated/site/types.gen.ts'
import { zSiteSignupSurveyInput } from '../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'

// The longest an "Other" text may be (the contract's limit).
const OTHER_MAX_LENGTH = 100

type Question = 'role' | 'companySize' | 'mainGoal' | 'heardFrom'

interface Step {
  question: Question
  required: boolean
  title: string
  options: { value: string; label: string }[]
}

type Answers = Partial<Record<Question, string>>

function choices(labels: Record<string, string>) {
  return Object.entries(labels).map(([value, label]) => ({ value, label }))
}

// The answers as the contract takes them: an `…Other` text is sent only with its `other` choice,
// and an unanswered optional question is left out.
function toInput(answers: Answers, others: Answers): SiteSignupSurveyInput {
  const withOther = (question: Question) => {
    const answer = answers[question]
    if (answer === undefined || answer === '') {
      return {}
    }
    return {
      [question]: answer,
      ...(answer === 'other' ? { [`${question}Other`]: (others[question] ?? '').trim() } : {}),
    }
  }
  return zSiteSignupSurveyInput.parse({
    ...withOther('role'),
    ...withOther('companySize'),
    ...withOther('mainGoal'),
    ...withOther('heardFrom'),
  })
}

// The Sign-up survey: four screens, one question each, answers sent once after the last.
// Nothing is kept before that, so a closed tab starts the survey again at the next sign-in.
export function SignupSurveyPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [index, setIndex] = useState(0)
  const [answers, setAnswers] = useState<Answers>({})
  const [others, setOthers] = useState<Answers>({})

  const steps: Step[] = [
    {
      question: 'role',
      required: true,
      title: t(($) => $.signupSurvey.role.title),
      options: choices({
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
      options: choices({
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
      options: choices({
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
      options: choices({
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
    invalidate: [siteUserGetMeQueryKey()],
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

  const step = steps[index]
  if (!step) {
    return null
  }
  const answer = answers[step.question]
  const otherText = others[step.question] ?? ''
  const isLast = index === steps.length - 1
  const complete =
    answer === undefined ? !step.required : answer !== 'other' || otherText.trim().length > 0

  const next = () => {
    if (isLast) {
      submit.mutate({ body: toInput(answers, others) })
    } else {
      setIndex(index + 1)
    }
  }

  return (
    <Stack maw={520} mx="auto" mt="xl">
      <Card withBorder padding="xl">
        <Stack>
          <Progress value={((index + 1) / steps.length) * 100} aria-hidden />
          <Text size="sm" c="dimmed">
            {t(($) => $.signupSurvey.progress, { current: index + 1, total: steps.length })}
          </Text>
          <Title order={3}>{step.title}</Title>
          <Radio.Group
            key={step.question}
            aria-label={step.title}
            value={answer ?? ''}
            onChange={(value) => setAnswers({ ...answers, [step.question]: value })}
          >
            <Stack gap="xs">
              {step.options.map((option) => (
                <Radio key={option.value} value={option.value} label={option.label} />
              ))}
            </Stack>
          </Radio.Group>
          {answer === 'other' && (
            <TextInput
              label={t(($) => $.signupSurvey.otherLabel)}
              maxLength={OTHER_MAX_LENGTH}
              value={otherText}
              onChange={(event) =>
                setOthers({ ...others, [step.question]: event.currentTarget.value })
              }
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
              onClick={() => setIndex(index - 1)}
            >
              {t(($) => $.signupSurvey.back)}
            </Button>
            <Button disabled={!complete} loading={submit.isPending} onClick={next}>
              {isLast ? t(($) => $.signupSurvey.finish) : t(($) => $.signupSurvey.next)}
            </Button>
          </Group>
        </Stack>
      </Card>
    </Stack>
  )
}
