import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: [
    'src/generated/**',
    'packages/analytics/src/generated/**',
    'internal/server/assets/**',
    'internal/emaildoc/assets/**',
  ],
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
      entry: ['i18next.config.ts'],
    },
    'packages/emaildoc': {
      // esbuild reads the entry and the shims; `editor-image-extension` is a virtual module that
      // build.mjs resolves to the editor's internal image chunk.
      entry: ['src/entry.mjs', 'src/shims.js'],
      ignoreDependencies: ['editor-image-extension'],
    },
  },
}

export default config
