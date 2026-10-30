import {
  type ClientMessage,
  type DemoInfo,
  loadResume,
  parseServerMessage,
  saveResume,
  terminalURL,
} from './protocol';
import { type DemoSession, type SessionEvents, TerminalView } from './view';

const encoder = new TextEncoder();

/**
 * One browser terminal attached to a TuiSense session on the server. The
 * terminal shows exactly the bytes the real TuiSense writes; this class
 * only moves bytes, sizes and a few control messages.
 */
export class TerminalSession implements DemoSession {
  private readonly view: TerminalView;
  private ws?: WebSocket;
  private finalEnd = false;
  private reconnects = 0;
  private reconnectTimer?: number;
  private disposed = false;

  constructor(
    container: HTMLElement,
    private readonly apiBase: string,
    private readonly events: SessionEvents,
  ) {
    this.view = new TerminalView(container, events);
    this.view.onInput = (d, binary) =>
      this.sendBytes(binary ? Uint8Array.from(d, (c) => c.charCodeAt(0) & 0xff) : encoder.encode(d));
    this.view.onResize = (cols, rows) => this.send({ type: 'resize', cols, rows });
  }

  get term() {
    return this.view.term;
  }

  async open(limits?: DemoInfo['limits']): Promise<void> {
    if (await this.view.open(limits)) this.connect(false);
  }

  applyFit() {
    this.view.applyFit();
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
          if (m.cols !== this.term.cols || m.rows !== this.term.rows) this.view.applyFit();
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
    this.view.focus();
  }

  screenText(): string {
    return this.view.screenText();
  }

  dispose() {
    this.disposed = true;
    window.clearTimeout(this.reconnectTimer);
    const ws = this.ws;
    this.ws = undefined;
    ws?.close();
    this.view.dispose();
  }
}
