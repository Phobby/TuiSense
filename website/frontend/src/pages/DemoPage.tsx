import { useCallback, useEffect, useRef, useState } from 'react';
import '@xterm/xterm/css/xterm.css';
import { API_BASE, BASE, DEMO_MODE, GITHUB_URL } from '../config';
import { commitURL, shortCommit, useDemoInfo } from '../info';
import { navigate } from '../router';
import { DEFAULT_LIMITS, endMessage } from '../terminal/protocol';
import { TerminalSession } from '../terminal/session';
import type { DemoSession, Status } from '../terminal/view';
import { WasmSession } from '../wasm/session';
import { IconBack, IconExitFullscreen, IconFullscreen, IconReset, IconRestart, Logo } from '../ui';

declare global {
  interface Window {
    /** End-to-end test hook (only with ?e2e in the URL). */
    __tuisenseDemo?: DemoSession;
  }
}

type KeyboardLock = { lock?: (keys?: string[]) => Promise<void>; unlock?: () => void };

function statusLabel(s: Status): string {
  switch (s.kind) {
    case 'connecting':
      return DEMO_MODE === 'wasm' ? 'Loading TuiSense…' : 'Starting your session…';
    case 'connected':
      return 'Connected';
    case 'reconnecting':
      return 'Reconnecting…';
    case 'exited':
      return 'TuiSense exited';
    case 'ended':
      return 'Session ended';
    case 'error':
      return 'Not connected';
  }
}

export function DemoPage() {
  const info = useDemoInfo();
  const frameRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<HTMLDivElement>(null);
  const sessionRef = useRef<DemoSession | undefined>(undefined);
  const [status, setStatus] = useState<Status>({ kind: 'connecting' });
  const [toast, setToast] = useState<{ text: string; id: number } | undefined>();
  const [focused, setFocused] = useState(true);
  const [tooSmall, setTooSmall] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const limits = info.status === 'online' ? info.info.limits : DEFAULT_LIMITS;
  const ready = info.status !== 'loading';

  useEffect(() => {
    if (!ready || !termRef.current) return;
    const events = {
      status: setStatus,
      toast: (text: string) => setToast({ text, id: Date.now() }),
      size: (_c: number, _r: number, small: boolean) => setTooSmall(small),
      focus: setFocused,
    };
    const commit = info.status === 'online' ? info.info.app.commit : '';
    const session: DemoSession =
      DEMO_MODE === 'wasm'
        ? new WasmSession(termRef.current, `${BASE}wasm/tuisense.wasm?v=${encodeURIComponent(commit || 'dev')}`, events)
        : new TerminalSession(termRef.current, API_BASE, events);
    sessionRef.current = session;
    if (new URLSearchParams(window.location.search).has('e2e')) window.__tuisenseDemo = session;
    void session.open(info.status === 'online' ? info.info.limits : undefined);
    return () => {
      session.dispose();
      sessionRef.current = undefined;
    };
    // The session lives as long as the page; info only seeds its limits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready]);

  useEffect(() => {
    if (!toast) return;
    const t = window.setTimeout(() => setToast(undefined), 4500);
    return () => window.clearTimeout(t);
  }, [toast]);

  useEffect(() => {
    const on = () => {
      const fs = document.fullscreenElement === frameRef.current;
      setFullscreen(fs);
      const kb = (navigator as Navigator & { keyboard?: KeyboardLock }).keyboard;
      // In fullscreen, keep Esc for TuiSense (hold Esc to leave fullscreen).
      if (fs) void kb?.lock?.(['Escape']).catch(() => undefined);
      else kb?.unlock?.();
      sessionRef.current?.applyFit();
      sessionRef.current?.focus();
    };
    document.addEventListener('fullscreenchange', on);
    return () => document.removeEventListener('fullscreenchange', on);
  }, []);

  const back = useCallback(() => {
    sessionRef.current?.end();
    navigate('/');
  }, []);
  const restart = () => {
    sessionRef.current?.restart();
    sessionRef.current?.focus();
  };
  const reset = () => {
    if (window.confirm('Reset the demo? TuiSense restarts and every setting, profile and report made in this session is deleted.')) {
      sessionRef.current?.reset();
    }
    sessionRef.current?.focus();
  };
  const toggleFullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void frameRef.current?.requestFullscreen({ navigationUI: 'hide' }).catch(() => setToast({ text: 'Fullscreen is not available here', id: Date.now() }));
  };

  const github = info.status === 'online' ? info.info.links.github || GITHUB_URL : GITHUB_URL;
  const app = info.status === 'online' ? info.info.app : undefined;
  const overlay = status.kind !== 'connected';

  return (
    <div className="demo">
      <header className="demo-bar">
        <button className="link-btn" onClick={back} title="End this session and return to the website">
          <IconBack />
          <Logo />
          <span className="hide-sm">Back to Website</span>
        </button>
        <div className={`status status-${status.kind}`} role="status" aria-live="polite">
          <span className="dot" />
          <span>{statusLabel(status)}</span>
        </div>
        {app && (
          <a className="version hide-sm" href={commitURL(github, app.commit)} target="_blank" rel="noreferrer" title="The exact build running in this demo">
            TuiSense v{app.version}
            {app.commit && <span> · {shortCommit(app.commit)}</span>}
          </a>
        )}
        <span className="spacer" />
        <button className="tool" onClick={restart} title="Restart TuiSense (keeps this session's settings)">
          <IconRestart />
          <span className="hide-sm">Restart Demo</span>
        </button>
        <button className="tool" onClick={reset} title="Restart with fresh session data">
          <IconReset />
          <span className="hide-sm">Reset Session</span>
        </button>
        <button className="tool" onClick={toggleFullscreen} title={fullscreen ? 'Exit fullscreen' : 'Fullscreen (hold Esc to leave)'}>
          {fullscreen ? <IconExitFullscreen /> : <IconFullscreen />}
          <span className="hide-sm">Fullscreen</span>
        </button>
      </header>

      {tooSmall && (
        <div className="banner">
          This window is smaller than TuiSense's minimum of {limits.minCols}×{limits.minRows} characters, so the terminal scrolls. Enlarge the window or use
          fullscreen for the full layout.
        </div>
      )}

      <main className="demo-main">
        <div className={`term-frame ${focused ? 'is-focused' : ''}`} ref={frameRef}>
          <div className="term" ref={termRef} onMouseDown={() => sessionRef.current?.focus()} />
          {fullscreen && (
            <button className="fs-exit" onClick={toggleFullscreen} title="Exit fullscreen (or hold Esc)">
              <IconExitFullscreen /> Exit fullscreen
            </button>
          )}
          {!focused && !overlay && (
            <button className="focus-hint" onClick={() => sessionRef.current?.focus()}>
              Click the terminal to type
            </button>
          )}
          {overlay && (
            <div className="overlay">
              <div className="overlay-card">
                {(status.kind === 'connecting' || status.kind === 'reconnecting') && (
                  <>
                    <div className="spinner" />
                    <p>
                      {status.kind === 'reconnecting'
                        ? 'Connection lost. Reconnecting to your session…'
                        : DEMO_MODE === 'wasm'
                          ? 'Loading TuiSense into your browser (about 4 MB, cached afterwards)…'
                          : 'Starting your private TuiSense session…'}
                    </p>
                  </>
                )}
                {status.kind === 'exited' && (
                  <>
                    <h2>TuiSense has exited</h2>
                    <p>The demo has no shell. Start TuiSense again to keep exploring.</p>
                    <div className="actions">
                      <button className="btn btn-primary" onClick={restart}>
                        <IconRestart /> Restart TuiSense
                      </button>
                      <button className="btn" onClick={back}>
                        Back to Website
                      </button>
                    </div>
                  </>
                )}
                {status.kind === 'ended' && (
                  <>
                    <h2>Session ended</h2>
                    <p>{endMessage(status.reason, limits)}</p>
                    <div className="actions">
                      <button className="btn btn-primary" onClick={() => sessionRef.current?.newSession()}>
                        Start a new session
                      </button>
                      <button className="btn" onClick={() => navigate('/')}>
                        Back to Website
                      </button>
                    </div>
                  </>
                )}
                {status.kind === 'error' && (
                  <>
                    <h2>{status.code === 'capacity' ? 'The demo is busy' : 'Could not start the demo'}</h2>
                    <p>{status.message}</p>
                    <div className="actions">
                      <button className="btn btn-primary" onClick={() => sessionRef.current?.newSession()}>
                        Try again
                      </button>
                      <button className="btn" onClick={() => navigate('/')}>
                        Back to Website
                      </button>
                    </div>
                  </>
                )}
              </div>
            </div>
          )}
        </div>
      </main>

      <footer className="demo-foot">
        <span>
          <strong>WEB DEMO — SIMULATED CONTROLLERS.</strong> Nothing here can access your hardware.
        </span>
        <span className="hide-sm">Shift+drag selects text · Ctrl/⌘+C copies a selection · Session data is deleted when the session ends</span>
      </footer>

      {toast && (
        <div className="toast" key={toast.id} role="status">
          {toast.text}
        </div>
      )}
    </div>
  );
}
