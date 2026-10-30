import type { DemoInfo } from '../terminal/protocol';
import { type DemoSession, type SessionEvents, TerminalView } from '../terminal/view';
import type { Snapshot } from './memfs';
import type { FromWorker, ToWorker } from './worker';

/**
 * TuiSense compiled to WebAssembly, running in a Web Worker in the visitor's
 * own browser. No server is involved: the page downloads the module once and
 * everything, including the session's files, stays in this tab.
 */
export class WasmSession implements DemoSession {
  private readonly view: TerminalView;
  private worker?: Worker;
  private exited = false;
  private snapshotWaiter?: (s: Snapshot | undefined) => void;
  private disposed = false;

  constructor(
    container: HTMLElement,
    private readonly wasmURL: string,
    private readonly events: SessionEvents,
  ) {
    this.view = new TerminalView(container, events);
    this.view.onInput = (data) => this.post({ type: 'input', data });
    this.view.onResize = (cols, rows) => this.post({ type: 'resize', cols, rows });
  }

  get term() {
    return this.view.term;
  }

  async open(limits?: DemoInfo['limits']): Promise<void> {
    if (await this.view.open(limits)) this.start();
  }

  applyFit() {
    this.view.applyFit();
  }

  private post(m: ToWorker) {
    this.worker?.postMessage(m);
  }

  private start(snapshot?: Snapshot) {
    if (this.disposed) return;
    this.events.status({ kind: 'connecting' });
    const w = new Worker(new URL('./worker.ts', import.meta.url), { type: 'module', name: 'tuisense' });
    this.worker = w;
    w.onmessage = (e: MessageEvent<FromWorker>) => {
      if (this.worker !== w) return;
      const m = e.data;
      switch (m.type) {
        case 'output':
          this.view.term.write(m.data);
          break;
        case 'running':
          this.events.status({ kind: 'connected' });
          this.view.focus();
          break;
        case 'exit':
          this.exited = true;
          // Drop TuiSense's screen and terminal modes (mouse reporting,
          // alternate screen) so nothing stale is left behind.
          this.view.term.reset();
          this.events.status({ kind: 'exited', code: m.code });
          break;
        case 'error':
          this.events.status({ kind: 'error', code: 'wasm', message: `TuiSense could not start in this browser: ${m.message}` });
          break;
        case 'snapshot':
          this.snapshotWaiter?.(m.data);
          break;
      }
    };
    this.exited = false;
    w.onerror = (e) => {
      if (this.worker === w && !this.exited) this.events.status({ kind: 'error', code: 'wasm', message: e.message || 'The WebAssembly worker failed.' });
    };
    this.post({ type: 'start', url: this.wasmURL, cols: this.view.term.cols, rows: this.view.term.rows, lang: navigator.language || 'en', snapshot });
  }

  /** The current worker's files, to carry into the next run. */
  private snapshot(): Promise<Snapshot | undefined> {
    if (!this.worker) return Promise.resolve(undefined);
    return new Promise((resolve) => {
      const t = window.setTimeout(() => resolve(undefined), 1000);
      this.snapshotWaiter = (s) => {
        window.clearTimeout(t);
        this.snapshotWaiter = undefined;
        resolve(s);
      };
      this.post({ type: 'snapshot' });
    });
  }

  private stop() {
    this.worker?.terminate();
    this.worker = undefined;
    this.view.term.reset();
  }

  restart() {
    void this.snapshot().then((s) => {
      this.stop();
      this.start(s);
    });
  }

  reset() {
    this.stop();
    this.start();
  }

  newSession() {
    this.reset();
  }

  end() {
    this.stop();
  }

  focus() {
    this.view.focus();
  }

  screenText(): string {
    return this.view.screenText();
  }

  dispose() {
    this.disposed = true;
    this.worker?.terminate();
    this.worker = undefined;
    this.view.dispose();
  }
}
