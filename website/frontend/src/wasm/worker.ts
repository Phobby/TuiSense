/// <reference lib="webworker" />
// Runs the real TuiSense, compiled to WebAssembly, inside a Web Worker.
//
// TuiSense is the same program as the native release (`tuisense
// --web-demo`). Limoni, its terminal engine, has a WebAssembly backend that
// writes terminal output through globalThis.__limoni_output and reads input
// and size through __limoni_input / __limoni_resize; this worker connects
// those to the page's xterm.js terminal with messages. Files live in an
// in-memory file system that disappears with the worker.
import { MemFS, type Snapshot } from './memfs';

export type ToWorker =
  | { type: 'start'; url: string; cols: number; rows: number; lang: string; snapshot?: Snapshot }
  | { type: 'input'; data: string }
  | { type: 'resize'; cols: number; rows: number }
  | { type: 'snapshot' };

export type FromWorker =
  | { type: 'output'; data: string }
  | { type: 'running' }
  | { type: 'exit'; code: number }
  | { type: 'error'; message: string }
  | { type: 'snapshot'; data: Snapshot };

declare const self: DedicatedWorkerGlobalScope;
type Bridge = {
  fs: MemFS;
  process: object;
  Go: new () => GoRuntime;
  __limoni_output?: (s: string) => void;
  __limoni_input?: (s: string) => void;
  __limoni_resize?: (w: number, h: number) => void;
};
interface GoRuntime {
  argv: string[];
  env: Record<string, string>;
  importObject: WebAssembly.Imports;
  exit: (code: number) => void;
  run(instance: WebAssembly.Instance): Promise<void>;
}

const g = globalThis as unknown as Bridge;
const HOME = '/home/demo';
const memfs = new MemFS((line) => console.log('[tuisense]', line));
g.fs = memfs;
g.process = {
  pid: 1,
  ppid: 0,
  getuid: () => 1000,
  getgid: () => 1000,
  geteuid: () => 1000,
  getegid: () => 1000,
  getgroups: () => [1000],
  umask: () => 0o077,
  cwd: () => HOME,
  chdir: () => {
    throw Object.assign(new Error('not supported'), { code: 'ENOSYS' });
  },
};

const post = (m: FromWorker) => self.postMessage(m);
let exited = false;
let pendingInput: string[] = [];

// After TuiSense exits, timers the Go runtime had scheduled may still fire
// and report "Go program has already exited"; that is not a failure.
self.addEventListener('error', (e) => {
  if (exited) e.preventDefault();
});
let size = { cols: 80, rows: 24 };

function send(data: string) {
  if (exited) return;
  if (g.__limoni_input) g.__limoni_input(data);
  else pendingInput.push(data);
}

/** Waits for Limoni to register its input bridge, then sends the real size. */
function connectWhenReady() {
  if (exited) return;
  if (!g.__limoni_resize || !g.__limoni_input) {
    setTimeout(connectWhenReady, 10);
    return;
  }
  g.__limoni_resize(size.cols, size.rows);
  for (const d of pendingInput) g.__limoni_input(d);
  pendingInput = [];
  post({ type: 'running' });
}

async function start(m: Extract<ToWorker, { type: 'start' }>) {
  size = { cols: m.cols, rows: m.rows };
  if (m.snapshot) memfs.restore(m.snapshot);
  for (const d of [`${HOME}/.config`, `${HOME}/.local/share`, `${HOME}/.cache`, '/tmp']) memfs.mkdirAll(d);
  // wasm_exec.js (Go's runtime glue) keeps an fs and process that already
  // exist, so it is loaded only now.
  await import('./wasm_exec.js');
  g.__limoni_output = (s: string) => post({ type: 'output', data: s });
  const go = new g.Go();
  let code = 0;
  go.exit = (c: number) => {
    code = c;
  };
  go.argv = ['tuisense', '--web-demo'];
  go.env = {
    HOME,
    XDG_CONFIG_HOME: `${HOME}/.config`,
    XDG_DATA_HOME: `${HOME}/.local/share`,
    XDG_CACHE_HOME: `${HOME}/.cache`,
    XDG_STATE_HOME: `${HOME}/.local/share`,
    TMPDIR: '/tmp',
    LANG: m.lang.toLowerCase().startsWith('tr') ? 'tr_TR.UTF-8' : 'C.UTF-8',
    TERM: 'xterm-256color',
    COLORTERM: 'truecolor',
  };
  const resp = await fetch(m.url);
  if (!resp.ok) throw new Error(`could not download TuiSense (${resp.status})`);
  const { instance } = await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
  connectWhenReady();
  await go.run(instance);
  exited = true;
  g.__limoni_input = g.__limoni_resize = undefined;
  post({ type: 'exit', code });
}

self.onmessage = (e: MessageEvent<ToWorker>) => {
  const m = e.data;
  switch (m.type) {
    case 'start':
      start(m).catch((err: unknown) => post({ type: 'error', message: err instanceof Error ? err.message : String(err) }));
      break;
    case 'input':
      send(m.data);
      break;
    case 'resize':
      size = { cols: m.cols, rows: m.rows };
      if (!exited) g.__limoni_resize?.(m.cols, m.rows);
      break;
    case 'snapshot':
      post({ type: 'snapshot', data: memfs.snapshot() });
      break;
  }
};
