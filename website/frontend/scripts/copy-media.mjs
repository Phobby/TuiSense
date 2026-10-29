// Copies the README's promotional recording of the native app into the
// site's public folder, so the website and the README share one file.
import { copyFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const src = resolve(here, '../../../docs/media/promo.gif');
const dst = resolve(here, '../public/media/promo.gif');
mkdirSync(dirname(dst), { recursive: true });
copyFileSync(src, dst);
