import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { ClipboardAddon, type IClipboardProvider, type ClipboardSelectionType } from '@xterm/addon-clipboard';
import { DEFAULT_LIMITS, type DemoInfo, type EndReason, fontSizeFor, routeKey, terminalSize } from './protocol';

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

/** What the demo page needs from a running TuiSense, wherever it runs. */
export interface DemoSession {
  readonly term: Terminal;
  open(limits?: DemoInfo['limits']): Promise<void>;
  applyFit(): void;
  /** Restarts TuiSense, keeping the session's settings. */
  restart(): void;
  /** Restarts TuiSense with fresh session data. */
  reset(): void;
  /** Starts a brand-new session after one ended or failed. */
  newSession(): void;
  /** Ends the session and deletes its data. */
  end(): void;
  focus(): void;
  /** Visible screen text (used by the end-to-end tests). */
  screenText(): string;
  dispose(): void;
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

/**
 * The browser terminal: xterm.js configured for TuiSense (fonts, true colour,
 * Unicode 11 widths, WebGL), its key routing, and its size. It knows nothing
 * about where TuiSense runs; a session wires its input and size to it.
 */
export class TerminalView {
  readonly term: Terminal;
  private readonly fit = new FitAddon();
  private limits: DemoInfo['limits'] = DEFAULT_LIMITS;
  private cols = 0;
  private rows = 0;
  private resizeTimer?: number;
  private observer?: ResizeObserver;
  private disposed = false;
  private readonly isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  /** Called with keyboard, mouse and paste input. */
  onInput: (data: string, binary: boolean) => void = () => {};
  /** Called when the grid size changes. */
  onResize: (cols: number, rows: number) => void = () => {};

  constructor(
    private readonly container: HTMLElement,
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

  /** Opens the terminal in the container once the fonts are loaded. */
  async open(limits?: DemoInfo['limits']): Promise<boolean> {
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
    if (this.disposed) return false;
    this.term.open(this.container);
    try {
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => webgl.dispose());
      this.term.loadAddon(webgl);
    } catch {
      // the DOM renderer still renders the same output
    }
    this.term.attachCustomKeyEventHandler((e) => this.onKey(e));
    this.term.onData((d) => this.onInput(d, false));
    this.term.onBinary((d) => this.onInput(d, true));
    this.term.textarea?.addEventListener('focus', () => this.events.focus(true));
    this.term.textarea?.addEventListener('blur', () => this.events.focus(false));
    this.observer = new ResizeObserver(() => this.scheduleFit());
    this.observer.observe(this.container);
    window.addEventListener('resize', this.onWindowResize);
    this.applyFit();
    return true;
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
      this.onResize(size.cols, size.rows);
    }
  }

  focus() {
    this.term.focus();
  }

  screenText(): string {
    const b = this.term.buffer.active;
    const lines: string[] = [];
    for (let i = 0; i < this.term.rows; i++) lines.push(b.getLine(b.viewportY + i)?.translateToString(true) ?? '');
    return lines.join('\n');
  }

  dispose() {
    this.disposed = true;
    window.clearTimeout(this.resizeTimer);
    window.removeEventListener('resize', this.onWindowResize);
    this.observer?.disconnect();
    this.term.dispose();
  }
}
