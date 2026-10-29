import { useEffect, useState } from 'react';
import { API_BASE } from './config';
import type { DemoInfo } from './terminal/protocol';

export type InfoState = { status: 'loading' } | { status: 'online'; info: DemoInfo } | { status: 'offline' };

/** Fetches /api/info: the deployed TuiSense version, commit and limits. */
export function useDemoInfo(): InfoState {
  const [state, setState] = useState<InfoState>({ status: 'loading' });
  useEffect(() => {
    const ctl = new AbortController();
    fetch(`${API_BASE}/api/info`, { signal: ctl.signal, cache: 'no-store' })
      .then((r) => (r.ok ? (r.json() as Promise<DemoInfo>) : Promise.reject(new Error(String(r.status)))))
      .then((info) => setState({ status: 'online', info }))
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
