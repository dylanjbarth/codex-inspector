import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

export default defineConfig(async () => {
  const { cloudflare } = await import('@cloudflare/vite-plugin')

  return {
    plugins: [
      react(),
      tailwindcss(),
      cloudflare({
        viteEnvironment: { name: 'server' },
        config: {
          main: './worker/index.ts',
          compatibility_date: '2026-07-22',
          assets: {
            binding: 'ASSETS',
            not_found_handling: 'single-page-application',
            run_worker_first: true,
          },
        },
      }),
    ],
    resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  }
})
