import { DOWNLOAD_URL, GITHUB_URL } from '../config';
import { commitURL, shortCommit, useDemoInfo } from '../info';
import { navigate } from '../router';
import { Logo, IconArrow, IconDownload, IconGitHub } from '../ui';

const models = [
  'Sony DualSense',
  'Sony DualSense Edge',
  'Xbox Wireless Controller',
  'Xbox Elite Series 2',
  'Sony DualShock 4',
  'Nintendo Switch Pro',
  '8BitDo Ultimate Bluetooth',
  'GameSir G7 SE',
  'Razer Wolverine V3 Pro',
  'MSI Force GC20 V2',
];

export function Landing() {
  const info = useDemoInfo();
  const github = info.status === 'online' ? info.info.links.github || GITHUB_URL : GITHUB_URL;
  const download = info.status === 'online' ? info.info.links.download || DOWNLOAD_URL : DOWNLOAD_URL;
  const launch = (e: React.MouseEvent) => {
    e.preventDefault();
    navigate('/demo');
  };

  return (
    <div className="site">
      <header className="nav">
        <a className="brand" href="/" aria-label="TuiSense home">
          <Logo />
          <span>TuiSense</span>
        </a>
        <nav>
          <a href="/demo" onClick={launch}>
            Live demo
          </a>
          <a href={github} target="_blank" rel="noreferrer">
            GitHub
          </a>
          <a href={download} target="_blank" rel="noreferrer">
            Download
          </a>
        </nav>
      </header>

      <main>
        <section className="hero">
          <div className={`pill pill-${info.status}`}>
            <span className="dot" />
            {info.status === 'online' && 'Live demo online · runs the real app'}
            {info.status === 'loading' && 'Checking the live demo…'}
            {info.status === 'offline' && 'Live demo unavailable right now'}
          </div>
          <h1>TuiSense</h1>
          <p className="tagline">Your Controller. Your Terminal.</p>
          <p className="lead">Experience TuiSense directly in your browser. No installation required.</p>
          <div className="actions">
            <a className="btn btn-primary" href="/demo" onClick={launch}>
              Launch Interactive Demo <IconArrow />
            </a>
            <a className="btn" href={github} target="_blank" rel="noreferrer">
              <IconGitHub /> View on GitHub
            </a>
            <a className="btn" href={download} target="_blank" rel="noreferrer">
              <IconDownload /> Download TuiSense
            </a>
          </div>
          <p className="fineprint">
            The browser demo runs the actual TuiSense application in an isolated sandbox with <strong>simulated controllers</strong>. It never
            asks for, reads or controls your hardware.
          </p>
        </section>

        <section className="showcase" aria-label="TuiSense recorded in a terminal">
          <div className="window">
            <div className="window-bar">
              <span />
              <span />
              <span />
              <em>tuisense</em>
            </div>
            <img src="/media/promo.gif" alt="TuiSense running in a terminal: 3D controller, diagnostics and settings" loading="lazy" />
          </div>
          <p className="caption">Recorded from the native terminal app. The live demo streams the same application.</p>
        </section>

        <section className="features">
          <article>
            <h3>The real app, not a mockup</h3>
            <p>
              Every character in the demo comes from the same Go and Limoni code as the native release: the dashboard, the 3D controller
              renderer, EZ and Advanced modes, diagnostics, health reports, settings and help.
            </p>
          </article>
          <article>
            <h3>Ten simulated controllers</h3>
            <p>Switch models, rotate them, inspect front and rear controls. Each model shows only the features it really has.</p>
            <ul className="models">
              {models.map((m) => (
                <li key={m}>{m}</li>
              ))}
            </ul>
          </article>
          <article>
            <h3>Isolated and temporary</h3>
            <p>
              Each visitor gets a private, sandboxed TuiSense process. Profiles, settings and reports exist only in your session and are deleted
              when it ends. No account, no tracking.
            </p>
          </article>
        </section>

        <section className="keys" aria-label="Getting around">
          <span>
            <kbd>?</kbd> Help
          </span>
          <span>
            <kbd>C</kbd> Choose a controller
          </span>
          <span>
            <kbd>E</kbd> EZ / Advanced
          </span>
          <span>
            <kbd>W</kbd>
            <kbd>A</kbd>
            <kbd>S</kbd>
            <kbd>D</kbd> Rotate
          </span>
          <span>
            <kbd>2</kbd> Diagnostics
          </span>
          <span>Mouse: click, drag, scroll</span>
        </section>
      </main>

      <footer className="footer">
        {info.status === 'online' ? (
          <span>
            Demo runs TuiSense v{info.info.app.version}
            {info.info.app.commit && (
              <>
                {' '}
                · commit{' '}
                <a href={commitURL(github, info.info.app.commit)} target="_blank" rel="noreferrer">
                  {shortCommit(info.info.app.commit)}
                </a>
              </>
            )}
          </span>
        ) : (
          <span>TuiSense</span>
        )}
        <span>
          MIT License ·{' '}
          <a href={github} target="_blank" rel="noreferrer">
            Source
          </a>
        </span>
      </footer>
    </div>
  );
}
