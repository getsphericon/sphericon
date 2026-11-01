import { defineConfig } from 'i18next-cli'

export default defineConfig({
  locales: ['en'],
  extract: {
    input: ['src/**/*.{ts,tsx}'],
    output: 'locales/{{language}}/{{namespace}}.json',
    defaultNS: 'translation',
    keySeparator: '.',
    // Keys the extractor cannot resolve: a lookup indexed by a value from the API, such as
    // t(($) => $.rules.firingStatus[firing.status]). Every other key the code no longer reads is
    // removed by `mise run fix` and fails `mise run check:i18n`. A new dynamic family goes here.
    preservePatterns: [
      'analytics.clicksByLink.automation',
      'analytics.clicksByLink.broadcast',
      'analytics.range.*',
      'automations.enrollmentStatus.*',
      'automations.failureKind.*',
      'broadcasts.hold.reasons.*',
      'capturedMessages.surfaces.*',
      'console.signupItems.kind.*',
      'console.workspaces.review.deleted',
      'console.workspaces.review.systemFrozen',
      'console.workspaces.review.warned',
      'customFields.types.*',
      'overview.attention.metrics.*',
      'rules.dryRunOutcome.*',
      'rules.firingStatus.*',
      'settings.members.source.*',
      'setupChecklist.steps.*.action',
      'signin.errors.*',
      'suppressions.reasons.*',
    ],
  },
  types: {
    input: 'locales/en/**/*.json',
    basePath: 'locales/en',
    output: 'types/i18next.d.ts',
    enableSelector: 'optimize',
  },
})
