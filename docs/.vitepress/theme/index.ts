import { theme, useOpenapi } from 'vitepress-openapi/client'

import 'vitepress-openapi/dist/style.css'
import DefaultTheme from 'vitepress/theme'

import spec from '../../../openapi/external.openapi.json'

useOpenapi({ spec })

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    theme.enhanceApp({ app })
  },
}
