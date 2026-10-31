import { defineConfig } from 'oxlint'

export default defineConfig({
  plugins: [
    'eslint',
    'typescript',
    'unicorn',
    'oxc',
    'react',
    'react-perf',
    'import',
    'jsdoc',
    'jsx-a11y',
    'node',
    'promise',
    'vitest',
    'jest',
    'nextjs',
    'vue',
  ],
  categories: {
    correctness: 'error',
    suspicious: 'error',
  },
  options: {
    typeAware: true,
  },
  env: {
    browser: true,
    es2026: true,
  },
  rules: {
    'typescript/no-explicit-any': 'error',
    'typescript/no-restricted-types': 'error',
    'typescript/no-misused-promises': 'error',
    'typescript/no-deprecated': 'error',
    'typescript/only-throw-error': [
      'error',
      {
        allow: [
          {
            from: 'package',
            package: '@tanstack/router-core',
            name: ['Redirect', 'NotFoundError'],
          },
        ],
      },
    ],
    'typescript/require-await': 'error',
    'typescript/prefer-nullish-coalescing': [
      'error',
      { ignorePrimitives: { string: true, boolean: true, number: true } },
    ],
    'typescript/prefer-optional-chain': 'error',
    'typescript/no-unnecessary-condition': 'error',
    'typescript/strict-void-return': 'error',
    'typescript/no-invalid-void-type': 'error',
    'typescript/no-import-type-side-effects': 'error',
    'typescript/consistent-type-definitions': 'error',
    'import/consistent-type-specifier-style': 'error',
    'import/no-cycle': 'error',
    'react/only-export-components': 'error',
    'unicorn/no-array-reduce': 'error',
    'unicorn/no-array-for-each': 'error',
    'eslint/complexity': 'error',
    'eslint/no-empty-function': 'error',
    'react/jsx-filename-extension': ['error', { extensions: ['.tsx'] }],
    'eslint/eqeqeq': 'error',
    'eslint/no-eq-null': 'error',
    'eslint/curly': 'error',
    'eslint/object-shorthand': 'error',
    'eslint/no-promise-executor-return': 'error',
    'eslint/no-use-before-define': ['error', { functions: false }],
    'react/display-name': 'error',
    'react/button-has-type': 'error',
    'react/jsx-no-useless-fragment': 'error',
    'react/no-object-type-as-default-prop': 'error',
    'unicorn/explicit-length-check': 'error',
    'unicorn/prefer-set-has': 'error',
    'unicorn/prefer-query-selector': 'error',
    'unicorn/prefer-dom-node-append': 'error',
    'unicorn/no-array-callback-reference': 'error',
    'unicorn/no-unnecessary-array-splice-count': 'error',
    // The only disabled rule: tsconfig uses jsx "react-jsx" (automatic runtime), where
    // importing React in scope is unnecessary; oxlint has no option to detect this.
    'react/react-in-jsx-scope': 'off',
    'eslint/no-underscore-dangle': ['error', { allow: ['_omq'] }],
    'import/no-unassigned-import': ['error', { allow: ['**/*.css', '**/i18n.ts'] }],
    'react/forbid-dom-props': ['error', { forbid: ['style', 'className'] }],
    'react/forbid-component-props': ['error', { forbid: ['style', 'className'] }],
    'react/forbid-elements': ['error', { forbid: ['style'] }],
    'eslint/no-restricted-imports': [
      'error',
      {
        paths: [
          {
            name: '@mantine/notifications',
            importNames: ['notifications', 'notificationsStore'],
            message:
              "Toasts live in the NotificationsProvider's own store: call `useNotify()` (src/hooks/useNotify.ts).",
          },
        ],
        patterns: [
          {
            group: [
              './*.css',
              '../*.css',
              '**/*.module.*',
              '#/**/*.css',
              '**/*.scss',
              '**/*.sass',
              '**/*.less',
            ],
            message: 'No custom CSS: style through Mantine props, theme and components.',
          },
          {
            group: ['styled-components', '@emotion/*', '@mantine/emotion'],
            message: 'No custom CSS: style through Mantine props, theme and components.',
          },
        ],
      },
    ],
  },
  overrides: [
    {
      files: ['src/**', 'packages/**'],
      rules: { 'react-hooks/rules-of-hooks': 'error' },
    },
    // Application code has named exports and reads its settings through the config module; the tool
    // configs at the repo root (vite, knip, oxlint, ...) must default-export and read process.env.
    {
      files: ['src/**', 'packages/*/src/**'],
      rules: { 'import/no-default-export': 'error', 'node/no-process-env': 'error' },
    },
    // Type-aware, so only where there are types; the runtime shims in packages/emaildoc are untyped
    // classic scripts, which is also why they cannot be asked to be modules.
    {
      files: ['**/*.ts', '**/*.tsx'],
      rules: { 'typescript/promise-function-async': 'error', 'import/unambiguous': 'error' },
    },
    // Text shown to a person comes from the catalog; the punctuation joining translated pieces is not text.
    {
      files: ['src/**'],
      rules: { 'react/jsx-no-literals': ['error', { allowedStrings: [':', '(', ')'] }] },
    },
    // The workflow builder's own stylesheet (a library's, not ours) carries global resets, so it is
    // rendered as a <style> while the builder is mounted and goes with it. The one file a <style>
    // element is allowed in.
    {
      files: ['src/pages/automations/AutomationBuilder.tsx'],
      rules: { 'react/forbid-elements': 'off' },
    },
  ],
  ignorePatterns: [
    'dist',
    'src/generated',
    'src/routeTree.gen.ts',
    'internal/contractcopy/testdata',
    'types/resources.d.ts',
    'packages/*/src/generated',
    'packages/*/dist',
    'internal/server/assets',
    'internal/emaildoc/assets',
    'internal/emaildoc/testdata/golden',
    'gen',
    'ent',
    '.agents',
    '.claude',
    '.cache',
    '.vitest',
    'tmp',
  ],
})
