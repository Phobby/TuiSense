// Static hosts (GitHub Pages) serve files, not routes: give /demo its own
// copy of the single-page app, and serve it for unknown paths too.
import { copyFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = resolve(dirname(fileURLToPath(import.meta.url)), '../dist');
mkdirSync(resolve(dist, 'demo'), { recursive: true });
copyFileSync(resolve(dist, 'index.html'), resolve(dist, 'demo/index.html'));
copyFileSync(resolve(dist, 'index.html'), resolve(dist, '404.html'));
