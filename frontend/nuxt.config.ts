// The studio is a local tool: there is no server to render on, every byte of
// data comes from the Go API at runtime, and the built output is copied into
// the Go image so the binary stays the only thing that runs.
export default defineNuxtConfig({
  compatibilityDate: '2026-09-20',
  ssr: false,
  devtools: { enabled: false },

  modules: ['@nuxt/ui', '@vueuse/nuxt'],
  css: ['~/assets/css/main.css'],

  // The build lands in .output/public and is copied into the Go image; writing
  // outside the project would put it beyond Docker's build context.
  nitro: {
    devProxy: {
      // On the host the studio is on localhost; inside compose it answers to
      // its service name, so the target is configurable.
      '/api': { target: `${process.env.STUDIO_API ?? 'http://localhost:8100'}/api`, changeOrigin: true },
      '/assets': { target: `${process.env.STUDIO_API ?? 'http://localhost:8100'}/assets`, changeOrigin: true },
    },
  },

  // There is no server and there may be no internet: the icons must live in the
  // client bundle, not be fetched from Iconify at runtime.
  icon: {
    serverBundle: false,
    clientBundle: { scan: true, includeCustomCollections: true },
  },

  ui: {
    // The score is dark by construction; a light mode would fight the
    // presentation window rather than help it.
    colorMode: false,
    theme: { colors: ['primary', 'error', 'warning', 'success', 'neutral'] },
  },

  app: {
    baseURL: '/',
    head: {
      title: 'Videolectura Studio UNAL',
      meta: [{ name: 'viewport', content: 'width=device-width, initial-scale=1' }],
    },
  },
})
