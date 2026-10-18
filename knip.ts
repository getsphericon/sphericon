import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: ['src/generated/**', 'packages/analytics/src/generated/**', 'internal/server/assets/**'],
  ignoreDependencies: [
    '@sphericon/analytics',
    '@jsonforms/core',
    '@jsonforms/react',
    '@typespec/.*',
    'npm-check-updates',
    // Run by `mise run generate:routes` (tsr generate), which knip cannot see.
    '@tanstack/router-cli',
  ],
  ignoreExportsUsedInFile: true,
  workspaces: {
    '.': {
      entry: [
        'i18next.config.ts',
        'docs/.vitepress/config.ts',
        'docs/.vitepress/theme/index.ts',
        'docs/api/*.paths.ts',
      ],
    },
  },
}

export default config
