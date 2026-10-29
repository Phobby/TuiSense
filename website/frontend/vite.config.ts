import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// In development the API (and the terminal WebSocket) is proxied to a local
// tuisense-webd, so the browser sees one origin as in production.
const backend = process.env.TUISENSE_WEB_DEV_BACKEND ?? 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: backend, ws: true },
      '/healthz': backend,
    },
  },
  build: {
    target: 'es2022',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    // Every asset is a file: the backend's Content-Security-Policy allows
    // fonts and scripts from the site itself only (no data: URLs).
    assetsInlineLimit: 0,
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
