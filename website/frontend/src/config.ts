// Build-time settings (Vite env). Defaults serve the API from the same
// origin as the website.
export const API_BASE: string = import.meta.env.VITE_API_BASE ?? '';
export const GITHUB_URL: string = import.meta.env.VITE_GITHUB_URL ?? 'https://github.com/Phobby/TuiSense';
export const DOWNLOAD_URL: string = import.meta.env.VITE_DOWNLOAD_URL ?? 'https://github.com/Phobby/TuiSense/releases/latest';
