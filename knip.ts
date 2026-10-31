import type { KnipConfig } from 'knip'

const config: KnipConfig = {
  ignore: [
    'src/generated/**',
    'packages/analytics/src/generated/**',
    'packages/node/src/generated/**',
    'internal/server/assets/**',
    'internal/emaildoc/assets/**',
    // Violating and clean examples for cmd/contractcopy's own tests.
    'internal/contractcopy/testdata/**',
  ],
  ignoreDependencies: [
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
      // The scripts that node runs: mise tasks, the CI workflows and the hook of .claude/settings.json.
      // The TypeSpec decorators, which `typespec/lib/main.tsp` imports.
      entry: ['i18next.config.ts', 'mise/scripts/*.ts', 'typespec/lib/decorators.js'],
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
