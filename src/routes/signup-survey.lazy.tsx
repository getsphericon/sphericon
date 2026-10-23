import { createLazyFileRoute } from '@tanstack/react-router'

import { SignupSurveyPage } from '../pages/auth/signup-survey.tsx'

export const Route = createLazyFileRoute('/signup-survey')({
  component: SignupSurveyPage,
})
