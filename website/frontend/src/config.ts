// Build-time settings (Vite env).
//
// VITE_DEMO_MODE selects where TuiSense runs:
//   wasm   — compiled to WebAssembly, in the visitor's browser (static
//            hosting such as GitHub Pages; no server)
//   server — one sandboxed process per visitor on tuisense-webd, over a
//            WebSocket (website/backend)
export const DEMO_MODE: 'wasm' | 'server' = import.meta.env.VITE_DEMO_MODE === 'wasm' ? 'wasm' : 'server';
// Where the site is served from ("/" or "/TuiSense/" on GitHub Pages).
export const BASE: string = import.meta.env.BASE_URL;
// Server mode: the API origin ("" = this site).
export const API_BASE: string = import.meta.env.VITE_API_BASE ?? '';
export const GITHUB_URL: string = import.meta.env.VITE_GITHUB_URL ?? 'https://github.com/Phobby/TuiSense';
export const DOWNLOAD_URL: string = import.meta.env.VITE_DOWNLOAD_URL ?? 'https://github.com/Phobby/TuiSense/releases/latest';
