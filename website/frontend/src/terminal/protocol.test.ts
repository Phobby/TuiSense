import { describe, expect, it } from 'vitest';
import {
  DEFAULT_LIMITS,
  endMessage,
  fontSizeFor,
  loadResume,
  parseServerMessage,
  routeKey,
  saveResume,
  terminalSize,
  terminalURL,
} from './protocol';

const key = (k: string, mods: Partial<Record<'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey', boolean>> = {}) => ({
  key: k,
  code: '',
  type: 'keydown',
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

describe('routeKey', () => {
  it('sends TuiSense its native keys', () => {
    for (const k of ['a', 'E', '?', '1', 'Tab', 'Enter', 'Escape', ' ', 'ArrowUp', 'PageDown', '[', ',']) {
      expect(routeKey(key(k), false, false)).toBe('terminal');
      expect(routeKey(key(k), false, true)).toBe('terminal');
    }
    expect(routeKey(key('Tab', { shiftKey: true }), false, false)).toBe('terminal');
    expect(routeKey(key('c', { ctrlKey: true }), false, false)).toBe('terminal'); // quits, as natively
  });
  it('keeps critical browser shortcuts', () => {
    expect(routeKey(key('F5'), false, false)).toBe('browser');
    expect(routeKey(key('F12'), false, false)).toBe('browser');
    expect(routeKey(key('r', { ctrlKey: true }), false, false)).toBe('browser');
    expect(routeKey(key('r', { metaKey: true }), false, true)).toBe('browser');
    expect(routeKey(key('I', { ctrlKey: true, shiftKey: true }), false, false)).toBe('browser');
    expect(routeKey(key('Tab', { ctrlKey: true }), false, false)).toBe('browser');
    expect(routeKey(key('w', { metaKey: true }), false, true)).toBe('browser');
  });
  it('lets the browser paste (bracketed paste reaches TuiSense)', () => {
    expect(routeKey(key('v', { ctrlKey: true }), false, false)).toBe('browser');
    expect(routeKey(key('V', { ctrlKey: true, shiftKey: true }), false, false)).toBe('browser');
    expect(routeKey(key('v', { metaKey: true }), false, true)).toBe('browser');
  });
  it('copies a selection', () => {
    expect(routeKey(key('c', { ctrlKey: true }), true, false)).toBe('copy');
    expect(routeKey(key('C', { ctrlKey: true, shiftKey: true }), true, false)).toBe('copy');
    expect(routeKey(key('c', { metaKey: true }), true, true)).toBe('copy');
  });
});

describe('terminalSize', () => {
  it('fits the container within the server bounds', () => {
    expect(terminalSize({ cols: 150, rows: 45 }, DEFAULT_LIMITS)).toEqual({ cols: 150, rows: 45, tooSmall: false });
    expect(terminalSize({ cols: 999, rows: 999 }, DEFAULT_LIMITS)).toEqual({ cols: 320, rows: 120, tooSmall: false });
  });
  it('never shrinks below the minimum; the frame scrolls instead', () => {
    expect(terminalSize({ cols: 30, rows: 8 }, DEFAULT_LIMITS)).toEqual({ cols: 40, rows: 12, tooSmall: true });
    expect(terminalSize(undefined, DEFAULT_LIMITS)).toEqual({ cols: 40, rows: 12, tooSmall: false });
  });
  it('uses readable font sizes', () => {
    expect(fontSizeFor(1920)).toBe(15);
    expect(fontSizeFor(1366)).toBe(14);
    expect(fontSizeFor(390)).toBe(11);
  });
});

describe('messages and URLs', () => {
  it('parses server messages', () => {
    expect(parseServerMessage('{"type":"exit","reason":"idle","final":true}')).toEqual({ type: 'exit', reason: 'idle', final: true });
    expect(parseServerMessage('nonsense')).toBeNull();
    expect(parseServerMessage('{"no":"type"}')).toBeNull();
  });
  it('builds the WebSocket URL', () => {
    expect(terminalURL('', { protocol: 'https:', host: 'demo.example' })).toBe('wss://demo.example/api/terminal');
    expect(terminalURL('', { protocol: 'http:', host: 'localhost:5173' })).toBe('ws://localhost:5173/api/terminal');
    expect(terminalURL('https://api.example', { protocol: 'https:', host: 'x' })).toBe('wss://api.example/api/terminal');
  });
  it('explains why a session ended', () => {
    expect(endMessage('idle', DEFAULT_LIMITS)).toContain('10 minutes');
    expect(endMessage('expired', DEFAULT_LIMITS)).toContain('30 minutes');
  });
});

describe('resume tokens', () => {
  class MemoryStorage {
    data = new Map<string, string>();
    get length() {
      return this.data.size;
    }
    clear() {
      this.data.clear();
    }
    getItem(k: string) {
      return this.data.get(k) ?? null;
    }
    key() {
      return null;
    }
    removeItem(k: string) {
      this.data.delete(k);
    }
    setItem(k: string, v: string) {
      this.data.set(k, v);
    }
  }
  it('round-trips valid tokens only', () => {
    const s = new MemoryStorage() as unknown as Storage;
    const v = { session: 'a'.repeat(32), token: 'b'.repeat(64) };
    saveResume(s, v);
    expect(loadResume(s)).toEqual(v);
    s.setItem('tuisense-demo-session', JSON.stringify({ session: '../etc', token: 'x' }));
    expect(loadResume(s)).toBeUndefined();
    saveResume(s, undefined);
    expect(loadResume(s)).toBeUndefined();
    expect(loadResume(undefined)).toBeUndefined();
  });
});
