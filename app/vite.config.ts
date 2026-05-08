import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import path from 'path'

export default defineConfig(() => {
  const basePath = process.env.VITE_BASE_PATH || ''
  const normalizedBase = basePath ? `${basePath}/` : '/'
  const proxyPrefix = basePath || ''

  return {
    base: normalizedBase,
    plugins: [
      tanstackRouter({ target: 'react', autoCodeSplitting: true }),
      react(),
      tailwindcss(),
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    optimizeDeps: {
      include: [
        '@tiptap/extension-table',
        '@tiptap/extension-table-row',
        '@tiptap/extension-table-header',
        '@tiptap/extension-table-cell',
      ],
    },
    define: {
      __BUILD_TIMESTAMP__: JSON.stringify(new Date().toISOString()),
    },
    server: {
      port: 3000,
      host: true,
      allowedHosts: true,
      proxy: {
        [`${proxyPrefix}/api`]: {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        [`${proxyPrefix}/public/sign`]: {
          target: 'http://localhost:8080',
          changeOrigin: true,
          // Only proxy XHR/fetch requests (not browser navigation)
          bypass(req) {
            if (req.headers.accept?.includes('text/html')) {
              return req.url // Skip proxy, let Vite serve the SPA
            }
          },
        },
        [`${proxyPrefix}/public/doc`]: {
          target: 'http://localhost:8080',
          changeOrigin: true,
          bypass(req) {
            if (req.headers.accept?.includes('text/html')) {
              return req.url
            }
          },
        },
        [`${proxyPrefix}/public/view`]: {
          target: 'http://localhost:8080',
          changeOrigin: true,
          bypass(req) {
            if (req.headers.accept?.includes('text/html')) {
              return req.url
            }
          },
        },
        [`${proxyPrefix}/health`]: {
          target: 'http://localhost:8080',
        },
        [`${proxyPrefix}/ready`]: {
          target: 'http://localhost:8080',
        },
        [`${proxyPrefix}/swagger`]: {
          target: 'http://localhost:8080',
        },
      },
    },
  }
})
