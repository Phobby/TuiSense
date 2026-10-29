import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { ClipboardAddon, type IClipboardProvider, type ClipboardSelectionType } from '@xterm/addon-clipboard';
import {
  DEFAULT_LIMITS,
  type ClientMessage,
  type DemoInfo,
  type EndReason,
  fontSizeFor,
  loadResume,
  parseServerMessage,
  routeKey,
  saveResume,
  terminalSize,
  terminalURL,
} from './protocol';

export type Status =
  | { kind: 'connecting' }
  | { kind: 'connected' }
  | { kind: 'reconnecting'; attempt: number }
  | { kind: 'exited'; code?: number }
  | { kind: 'ended'; reason: EndReason }
  | { kind: 'error'; code: string; message: string };

export interface SessionEvents {
  status(s: Status): void;
  toast(message: string): void;
  size(cols: number, rows: number, tooSmall: boolean): void;
  focus(focused: boolean): void;
}

// Colours of TuiSense's own canvas (internal/theme), so the few cells the
// application leaves unpainted match it.
const theme = {
  background: '#0a0d16',
  foreground: '#eef2ff',
  cursor: '#0a0d16',
  cursorAccent: '#0a0d16',
  selectionBackground: 'rgba(139, 92, 246, 0.38)',
  black: '#0a0d16',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#facc15',
  blue: '#5b8cff',
  magenta: '#9b7bff',
  cyan: '#829cdc',
  white: '#cbd5e1',
  brightBlack: '#475569',
  brightRed: '#fca5a5',
  brightGreen: '#86efac',
  brightYellow: '#fde68a',
  brightBlue: '#93b4ff',
  brightMagenta: '#c4b5fd',
  brightCyan: '#a5b8e8',
  brightWhite: '#f8fafc',
};

export const FONT_FAMILY = '"JetBrains Mono", "TuiSense Braille", "DejaVu Sans Mono", Menlo, Consolas, "Liberation Mono", monospace';

/**
 * OSC 52 clipboard for TuiSense's copy actions: writes reach the visitor's
 * clipboard (the browser may ask for permission); reads are refused, so the
 * remote application can never see what the visitor has copied.
 */
class WriteOnlyClipboard implements IClipboardProvider {
  constructor(private readonly onCopied: () => void) {}
  readText(_selection: ClipboardSelectionType): string {
    return '';
  }
  async writeText(_selection: ClipboardSelectionType, text: string): Promise<void> {
    if (!text) return;
    await navigator.clipboard.writeText(text);
    this.onCopied();
  }
}

const encoder = new TextEncoder();

/**
 * One browser terminal attached to a TuiSense session on the server. The
 * terminal shows exactly the bytes the real TuiSense writes; this class
 * only moves bytes, sizes and a few control messages.
 */
export class TerminalSession {
  readonly term: Terminal;
  private readonly fit = new FitAddon();
  private ws?: WebSocket;
  private limits: DemoInfo['limits'] = DEFAULT_LIMITS;
  private cols = 0;
  private rows = 0;
  private finalEnd = false;
  private reconnects = 0;
  private reconnectTimer?: number;
  private resizeTimer?: number;
  private observer?: ResizeObserver;
  private disposed = false;
  private readonly isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

  constructor(
    private readonly container: HTMLElement,
    private readonly apiBase: string,
    private readonly events: SessionEvents,
  ) {
    this.term = new Terminal({
      allowProposedApi: true, // Unicode 11 widths
      fontFamily: FONT_FAMILY,
      fontSize: fontSizeFor(window.innerWidth),
      lineHeight: 1,
      letterSpacing: 0,
      scrollback: 0, // TuiSense uses the alternate screen
      cursorBlink: false,
      cursorStyle: 'block',
      cursorInactiveStyle: 'none',
      macOptionIsMeta: true,
      macOptionClickForcesSelection: true,
      drawBoldTextInBrightColors: false,
      minimumContrastRatio: 1,
      customGlyphs: true, // pixel-exact box drawing and block elements
      theme,
    });
    this.term.loadAddon(this.fit);
    const unicode = new Unicode11Addon();
    this.term.loadAddon(unicode);
    this.term.unicode.activeVersion = '11';
    this.term.loadAddon(new ClipboardAddon(undefined, new WriteOnlyClipboard(() => this.events.toast('Copied to your clipboard'))));
  }

  /** Opens the terminal in the container and connects. */
  async open(limits?: DemoInfo['limits']): Promise<void> {
    if (limits) this.limits = limits;
    // Cell metrics are measured when the terminal opens: load the fonts first.
    try {
      const size = `${this.term.options.fontSize}px`;
      await Promise.all([
        document.fonts.load(`${size} "JetBrains Mono"`),
        document.fonts.load(`700 ${size} "JetBrains Mono"`),
        document.fonts.load(`${size} "TuiSense Braille"`, '⣿'),
      ]);
    } catch {
      // fall back to the system monospace font
    }
    if (this.disposed) return;
    this.term.open(this.container);
    try {
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => webgl.dispose());
      this.term.loadAddon(webgl);
    } catch {
      // the DOM renderer still renders the same output
    }
    this.term.attachCustomKeyEventHandler((e) => this.onKey(e));
    this.term.onData((d) => this.sendBytes(encoder.encode(d)));
    this.term.onBinary((d) => this.sendBytes(Uint8Array.from(d, (c) => c.charCodeAt(0) & 0xff)));
    this.term.textarea?.addEventListener('focus', () => this.events.focus(true));
    this.term.textarea?.addEventListener('blur', () => this.events.focus(false));
    this.observer = new ResizeObserver(() => this.scheduleFit());
    this.observer.observe(this.container);
    window.addEventListener('resize', this.onWindowResize);
    this.applyFit();
    this.connect(false);
  }

  private onWindowResize = () => {
    const fs = fontSizeFor(window.innerWidth);
    if (fs !== this.term.options.fontSize) this.term.options.fontSize = fs;
    this.scheduleFit();
  };

  private onKey(e: KeyboardEvent): boolean {
    const route = routeKey(e, this.term.hasSelection(), this.isMac);
    if (route === 'copy') {
      if (e.type === 'keydown') {
        void navigator.clipboard.writeText(this.term.getSelection()).then(
          () => this.events.toast('Selection copied'),
          () => this.events.toast('The browser did not allow copying'),
        );
        this.term.clearSelection();
      }
      e.preventDefault();
      return false;
    }
    return route === 'terminal';
  }

  private scheduleFit() {
    window.clearTimeout(this.resizeTimer);
    this.resizeTimer = window.setTimeout(() => this.applyFit(), 60);
  }

  /** Sizes the terminal to its frame (never below TuiSense's minimum). */
  applyFit() {
    if (this.disposed || !this.term.element) return;
    const size = terminalSize(this.fit.proposeDimensions(), this.limits);
    if (size.cols !== this.term.cols || size.rows !== this.term.rows) {
      this.term.resize(size.cols, size.rows);
    }
    this.events.size(size.cols, size.rows, size.tooSmall);
    if (size.cols !== this.cols || size.rows !== this.rows) {
      this.cols = size.cols;
      this.rows = size.rows;
      this.send({ type: 'resize', cols: size.cols, rows: size.rows });
    }
  }

  private send(m: ClientMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(m));
  }

  private sendBytes(b: Uint8Array) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(b);
  }

  private connect(isReconnect: boolean) {
    if (this.disposed) return;
    this.finalEnd = false;
    this.events.status(isReconnect ? { kind: 'reconnecting', attempt: this.reconnects } : { kind: 'connecting' });
    const ws = new WebSocket(terminalURL(this.apiBase, window.location));
    ws.binaryType = 'arraybuffer';
    this.ws = ws;
    ws.onopen = () => {
      this.cols = this.term.cols;
      this.rows = this.term.rows;
      ws.send(
        JSON.stringify({
          type: 'start',
          cols: this.term.cols,
          rows: this.term.rows,
          lang: navigator.language || 'en',
          resume: loadResume(window.sessionStorage),
        } satisfies ClientMessage),
      );
    };
    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        this.term.write(new Uint8Array(ev.data));
        return;
      }
      const m = parseServerMessage(String(ev.data));
      if (!m) return;
      switch (m.type) {
        case 'ready':
          this.reconnects = 0;
          saveResume(window.sessionStorage, { session: m.session, token: m.token });
          if (!m.resumed) this.term.reset();
          this.events.status(m.running ? { kind: 'connected' } : { kind: 'exited' });
          if (m.cols !== this.term.cols || m.rows !== this.term.rows) this.applyFit();
          this.term.focus();
          break;
        case 'started':
          this.term.reset();
          this.events.status({ kind: 'connected' });
          this.term.focus();
          break;
        case 'exit':
          // TuiSense is gone: drop its screen and terminal modes (mouse
          // reporting, alternate screen) so nothing stale is left behind.
          this.term.reset();
          if (m.final || m.reason !== 'exited') {
            this.finalEnd = true;
            saveResume(window.sessionStorage, undefined);
            this.events.status({ kind: 'ended', reason: m.reason });
          } else {
            this.events.status({ kind: 'exited', code: m.code });
          }
          break;
        case 'error':
          this.finalEnd = true;
          saveResume(window.sessionStorage, undefined);
          this.events.status({ kind: 'error', code: m.code, message: m.message });
          break;
        case 'warning':
          this.events.toast(
            m.reason === 'idle'
              ? `No input for a while: the session ends in ${m.seconds} s unless you press a key.`
              : `This demo session reaches its time limit in ${Math.round(m.seconds / 60)} min.`,
          );
          break;
        case 'notice':
          this.events.toast(m.message);
          break;
      }
    };
    ws.onclose = () => {
      if (this.ws !== ws || this.disposed || this.finalEnd) return;
      // A dropped connection: the server keeps the session for a short
      // grace period, so reconnect with the saved token.
      if (this.reconnects >= 6 || !loadResume(window.sessionStorage)) {
        this.events.status({ kind: 'error', code: 'connection', message: 'The connection to the demo server was lost.' });
        return;
      }
      this.reconnects++;
      const delay = Math.min(8000, 500 * 2 ** (this.reconnects - 1));
      this.reconnectTimer = window.setTimeout(() => this.connect(true), delay);
    };
  }

  /** Restarts TuiSense in the same session (settings are kept). */
  restart() {
    if (this.ws?.readyState === WebSocket.OPEN && !this.finalEnd) this.send({ type: 'restart' });
    else this.newSession();
  }

  /** Restarts TuiSense with fresh session data. */
  reset() {
    if (this.ws?.readyState === WebSocket.OPEN && !this.finalEnd) this.send({ type: 'reset' });
    else this.newSession();
  }

  /** Starts a brand-new session (after one ended or failed). */
  newSession() {
    window.clearTimeout(this.reconnectTimer);
    saveResume(window.sessionStorage, undefined);
    this.reconnects = 0;
    const old = this.ws;
    this.ws = undefined;
    old?.close();
    this.term.reset();
    this.connect(false);
  }

  /** Ends the session on the server (its process and data are deleted). */
  end() {
    this.finalEnd = true;
    this.send({ type: 'close' });
    saveResume(window.sessionStorage, undefined);
  }

  focus() {
    this.term.focus();
  }

  /** Visible screen text (used by the end-to-end tests). */
  screenText(): string {
    const b = this.term.buffer.active;
    const lines: string[] = [];
    for (let i = 0; i < this.term.rows; i++) lines.push(b.getLine(b.viewportY + i)?.translateToString(true) ?? '');
    return lines.join('\n');
  }

  dispose() {
    this.disposed = true;
    window.clearTimeout(this.reconnectTimer);
    window.clearTimeout(this.resizeTimer);
    window.removeEventListener('resize', this.onWindowResize);
    this.observer?.disconnect();
    const ws = this.ws;
    this.ws = undefined;
    ws?.close();
    this.term.dispose();
  }
}
