import { defineConfig } from 'i18next-cli'

export default defineConfig({
  locales: ['en'],
  extract: {
    input: ['src/**/*.{ts,tsx}'],
    output: 'locales/{{language}}/{{namespace}}.json',
    defaultNS: 'translation',
    keySeparator: '.',
    // Keep keys the extractor can't see statically (e.g. dynamic lookups like
    // t(($) => $.status[status])); we maintain those by hand.
    removeUnusedKeys: false,
  },
  types: {
    input: 'locales/en/**/*.json',
    basePath: 'locales/en',
    output: 'types/i18next.d.ts',
    enableSelector: 'optimize',
  },
})
