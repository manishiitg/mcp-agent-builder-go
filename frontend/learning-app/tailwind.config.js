// Tailwind for the shared chat transcript the learning app hosts in platform
// mode. The theme (shadcn colour tokens, radii, animations) is AgentWorks'
// own so the components look the same here; preflight is OFF because the
// learning app has its own hand-written styles and a browser reset would
// change every one of them.
import base from '../tailwind.config.js'

export default {
  ...base,
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}', '../src/**/*.{ts,tsx}', '../shared/**/*.{ts,tsx}'],
  corePlugins: { preflight: false },
}
