import { useEffect, useState } from 'react';
import { API_BASE, BASE, DEMO_MODE, DOWNLOAD_URL, GITHUB_URL } from './config';
import { DEFAULT_LIMITS, type DemoInfo } from './terminal/protocol';

export type InfoState = { status: 'loading' } | { status: 'online'; info: DemoInfo } | { status: 'offline' };

/** The WebAssembly build's identity, written next to it by make web-wasm. */
export interface WasmBuild {
  version: string;
  commit: string;
  file: string;
}

export const wasmBuildURL = () => `${BASE}wasm/build.json`;

/**
 * The deployed TuiSense version, commit and limits: /api/info from the
 * backend, or the WebAssembly build's build.json.
 */
export function useDemoInfo(): InfoState {
  const [state, setState] = useState<InfoState>({ status: 'loading' });
  useEffect(() => {
    const ctl = new AbortController();
    const url = DEMO_MODE === 'wasm' ? wasmBuildURL() : `${API_BASE}/api/info`;
    fetch(url, { signal: ctl.signal, cache: 'no-store' })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((data: DemoInfo | WasmBuild) => {
        if (DEMO_MODE !== 'wasm') {
          setState({ status: 'online', info: data as DemoInfo });
          return;
        }
        const b = data as WasmBuild;
        setState({
          status: 'online',
          info: {
            app: { version: b.version, commit: b.commit },
            backend: { version: b.version, commit: b.commit },
            sessions: { active: 0, max: 0 },
            limits: DEFAULT_LIMITS,
            links: { github: GITHUB_URL, download: DOWNLOAD_URL },
          },
        });
      })
      .catch((e: unknown) => {
        if (!(e instanceof DOMException && e.name === 'AbortError')) setState({ status: 'offline' });
      });
    return () => ctl.abort();
  }, []);
  return state;
}

export function shortCommit(c: string | undefined): string {
  if (!c) return '';
  return c.replace(/-dirty$/, '').slice(0, 7) + (c.endsWith('-dirty') ? '-dirty' : '');
}

export function commitURL(github: string, commit: string): string | undefined {
  const sha = commit.replace(/-dirty$/, '');
  return /^[0-9a-f]{7,40}$/.test(sha) ? `${github.replace(/\/$/, '')}/commit/${sha}` : undefined;
}
