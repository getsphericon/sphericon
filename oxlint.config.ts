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
  ignorePatterns: [
    'dist',
    'src/generated',
    'src/routeTree.gen.ts',
    'types/resources.d.ts',
    'packages/*/src/generated',
    'packages/*/dist',
    'internal/server/assets',
    'gen',
    'ent',
    '.agents',
    '.claude',
    '.cache',
    '.vitest',
    'tmp',
  ],
})
