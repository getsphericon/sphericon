import { RuleConfigSeverity } from '@commitlint/types'
import type { UserConfig } from '@commitlint/types'

// Conventional Commits drive release-please (versioning and changelog), so every
// commit on main must parse. Header limit is raised from the 100-char default to fit
// the project's descriptive scopes.
export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'header-max-length': [RuleConfigSeverity.Error, 'always', 120],
  },
} satisfies UserConfig
