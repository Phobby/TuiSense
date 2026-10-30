import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Go's wasm_exec.js is copied from the Go toolchain by `make web-wasm`. A
// server-mode build never loads it, so a placeholder stands in when it is
// absent (for example in the frontend Docker image, which has no Go).
const wasmExec = fileURLToPath(new URL('./src/wasm/wasm_exec.js', import.meta.url));
const haveWasmExec = existsSync(wasmExec);
if (process.env.VITE_DEMO_MODE === 'wasm' && !haveWasmExec) {
  throw new Error('src/wasm/wasm_exec.js is missing: run `make web-wasm` first');
}

// In development the API (and the terminal WebSocket) is proxied to a local
// tuisense-webd, so the browser sees one origin as in production.
const backend = process.env.TUISENSE_WEB_DEV_BACKEND ?? 'http://127.0.0.1:8080';

export default defineConfig({
  // "/TuiSense/" for a GitHub Pages project site (VITE_BASE), else "/".
  base: process.env.VITE_BASE ?? '/',
  plugins: [react()],
  resolve: {
    alias: haveWasmExec ? {} : { './wasm_exec.js': fileURLToPath(new URL('./src/wasm/wasm_exec.stub.js', import.meta.url)) },
  },
  // The WebAssembly worker loads Go's runtime glue with a dynamic import.
  worker: { format: 'es' },
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
