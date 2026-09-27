// Nuxt 4 admin SPA for liapi.
// Builds a fully static SPA into server/adminui so the Go binary can embed it.
import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  ssr: false,
  devtools: { enabled: false },

  // fuxsto-design ships its own compiled styles; only tailwindcss is needed.
  css: [
    '~/assets/css/main.css',
  ],

  // Static SPA: every route falls back to index.html.
  router: {
    options: {
      hashMode: false,
    },
  },

  nitro: {
    preset: 'static',
    output: {
      publicDir: '../server/adminui',
    },
    // Dev-only: proxy admin API + metrics to a locally running liapi (:8787)
    // so `npm run dev` works against the real backend.
    devProxy: {
      '/admin/api': { target: 'http://127.0.0.1:8787', changeOrigin: true },
      '/v1': { target: 'http://127.0.0.1:8787', changeOrigin: true },
      '/metrics': { target: 'http://127.0.0.1:8787', changeOrigin: true },
    },
  },

  app: {
    baseURL: '/',
    pageTransition: { name: 'page', mode: 'out-in' },
    head: {
      title: 'Liapi Admin',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'color-scheme', content: 'dark light' },
      ],
      htmlAttrs: { lang: 'zh-CN' },
    },
  },

  vite: {
    plugins: [tailwindcss()],
    build: {
      // Keep the embedded asset footprint small and predictable.
      chunkSizeWarningLimit: 1500,
    },
  },

  typescript: {
    strict: true,
    typeCheck: false,
  },
})
