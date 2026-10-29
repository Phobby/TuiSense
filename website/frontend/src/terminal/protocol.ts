// Wire protocol of the web demo terminal (see website/backend/server).
//
// Binary WebSocket frames carry terminal bytes in both directions: the real
// TuiSense output to the browser, keyboard/mouse/paste input to TuiSense.
// Text frames carry these JSON control messages.

export interface StartMessage {
  type: 'start';
  cols: number;
  rows: number;
  lang: string;
  resume?: { session: string; token: string };
}

export type ClientMessage =
  | StartMessage
  | { type: 'resize'; cols: number; rows: number }
  | { type: 'restart' }
  | { type: 'reset' }
  | { type: 'close' };

export type EndReason = 'exited' | 'idle' | 'expired' | 'disconnected' | 'replaced' | 'shutdown' | 'closed' | 'error';

export type ServerMessage =
  | { type: 'ready'; session: string; token: string; resumed: boolean; running: boolean; cols: number; rows: number; expiresIn: number; idleAfter: number }
  | { type: 'exit'; reason: EndReason; code?: number; final?: boolean }
  | { type: 'started'; reset: boolean }
  | { type: 'error'; code: string; message: string }
  | { type: 'warning'; reason: 'idle' | 'expired'; seconds: number }
  | { type: 'notice'; message: string };

export function parseServerMessage(data: string): ServerMessage | null {
  try {
    const m = JSON.parse(data) as { type?: unknown };
    if (m && typeof m.type === 'string') return m as ServerMessage;
  } catch {
    // ignore malformed frames
  }
  return null;
}

/** Public demo information from GET /api/info. */
export interface DemoInfo {
  app: { version: string; commit: string };
  backend: { version: string; commit: string };
  sessions: { active: number; max: number };
  limits: { idleTimeout: number; maxDuration: number; minCols: number; minRows: number; maxCols: number; maxRows: number };
  links: { github: string; download: string };
}

export const DEFAULT_LIMITS: DemoInfo['limits'] = {
  idleTimeout: 600,
  maxDuration: 1800,
  minCols: 40,
  minRows: 12,
  maxCols: 320,
  maxRows: 120,
};

/**
 * Terminal size for a container: the largest grid that fits, but never
 * below the minimum TuiSense needs (the frame then scrolls instead of
 * squeezing character cells) and never above the server's maximum.
 */
export function terminalSize(
  fit: { cols: number; rows: number } | undefined,
  limits: Pick<DemoInfo['limits'], 'minCols' | 'minRows' | 'maxCols' | 'maxRows'>,
): { cols: number; rows: number; tooSmall: boolean } {
  const cols = Math.floor(fit?.cols ?? limits.minCols);
  const rows = Math.floor(fit?.rows ?? limits.minRows);
  return {
    cols: Math.min(Math.max(cols, limits.minCols), limits.maxCols),
    rows: Math.min(Math.max(rows, limits.minRows), limits.maxRows),
    tooSmall: cols < limits.minCols || rows < limits.minRows,
  };
}

/** Font size (CSS px) for the terminal at a viewport width. */
export function fontSizeFor(viewportWidth: number): number {
  if (viewportWidth >= 1800) return 15;
  if (viewportWidth >= 1280) return 14;
  if (viewportWidth >= 960) return 13;
  if (viewportWidth >= 640) return 12;
  return 11;
}

export type KeyRoute = 'terminal' | 'browser' | 'copy';

/**
 * Decides who gets a key press while the terminal has focus. TuiSense gets
 * everything it uses natively (letters, digits, arrows, Tab, Enter, Esc,
 * Space, PgUp/PgDn, Ctrl+C to quit). The browser keeps reload, developer
 * tools, its own fullscreen toggle and paste, and Cmd shortcuts on macOS.
 * Copy shortcuts copy the mouse selection (Shift+drag) when there is one.
 */
export function routeKey(
  e: Pick<KeyboardEvent, 'key' | 'code' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey' | 'type'>,
  hasSelection: boolean,
  isMac: boolean,
): KeyRoute {
  const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
  const primary = isMac ? e.metaKey : e.ctrlKey;
  if (primary && key === 'c' && hasSelection) return 'copy';
  if (e.ctrlKey && e.shiftKey && key === 'c' && hasSelection) return 'copy';
  if (key === 'F5' || key === 'F11' || key === 'F12') return 'browser';
  if (primary && (key === 'v' || key === 'r')) return 'browser';
  if (e.ctrlKey && e.shiftKey && (key === 'v' || key === 'i' || key === 'j' || key === 'c' || key === 'r')) return 'browser';
  if (e.ctrlKey && (key === 'Tab' || key === 'PageUp' || key === 'PageDown')) return 'browser';
  if (isMac && e.metaKey) return 'browser';
  return 'terminal';
}

/** Saved reconnect credentials for this browser tab. */
export interface ResumeToken {
  session: string;
  token: string;
}

const RESUME_KEY = 'tuisense-demo-session';

export function loadResume(storage: Storage | undefined): ResumeToken | undefined {
  try {
    const raw = storage?.getItem(RESUME_KEY);
    if (!raw) return undefined;
    const v = JSON.parse(raw) as Partial<ResumeToken>;
    if (typeof v.session === 'string' && /^[0-9a-f]{32}$/.test(v.session) && typeof v.token === 'string' && /^[0-9a-f]{64}$/.test(v.token)) {
      return { session: v.session, token: v.token };
    }
  } catch {
    // storage unavailable or corrupted
  }
  return undefined;
}

export function saveResume(storage: Storage | undefined, v: ResumeToken | undefined): void {
  try {
    if (v) storage?.setItem(RESUME_KEY, JSON.stringify(v));
    else storage?.removeItem(RESUME_KEY);
  } catch {
    // storage unavailable
  }
}

/** The terminal WebSocket URL for an API base ("" = this site). */
export function terminalURL(apiBase: string, location: Pick<Location, 'protocol' | 'host'>): string {
  if (apiBase) {
    const u = new URL('/api/terminal', apiBase);
    u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
    return u.toString();
  }
  return `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/terminal`;
}

/** Human explanation of why a session ended. */
export function endMessage(reason: EndReason, limits: DemoInfo['limits']): string {
  const minutes = (s: number) => Math.max(1, Math.round(s / 60));
  switch (reason) {
    case 'idle':
      return `The session ended after ${minutes(limits.idleTimeout)} minutes without input. Its temporary data was deleted.`;
    case 'expired':
      return `Demo sessions last up to ${minutes(limits.maxDuration)} minutes. Its temporary data was deleted.`;
    case 'replaced':
      return 'This session was opened in another tab.';
    case 'shutdown':
      return 'The demo server is restarting. Please start a new session in a moment.';
    case 'disconnected':
      return 'The connection was lost for too long and the session was closed.';
    case 'closed':
      return 'The session was closed and its temporary data deleted.';
    default:
      return 'The session ended.';
  }
}
