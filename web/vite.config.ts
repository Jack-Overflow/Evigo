import { defineConfig } from 'vitest/config';
import { readFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'keep-embed-directory',
      generateBundle() {
        this.emitFile({
          type: 'asset',
          fileName: '.gitignore',
          source: '*\n!.gitignore\n',
        });
        this.emitFile({
          type: 'asset',
          fileName: 'licenses.txt',
          source: readFileSync(
            new URL('../THIRD_PARTY_LICENSES', import.meta.url),
            'utf8',
          ),
        });
      },
    },
  ],
  build: { outDir: '../internal/api/static', emptyOutDir: true },
  preview: {
    headers: {
      'Content-Security-Policy':
        "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
        configure(proxy) {
          proxy.on('proxyReq', (request) =>
            request.setHeader('Origin', 'http://127.0.0.1:8080'),
          );
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
  },
});
