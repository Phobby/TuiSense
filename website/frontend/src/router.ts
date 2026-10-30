import { useEffect, useState } from 'react';
import { BASE } from './config';

/** The page path below the site's base: "/" or "/demo". */
export function pagePath(pathname: string, base: string): string {
  const b = base.replace(/\/$/, '');
  const p = b && pathname.startsWith(b) ? pathname.slice(b.length) : pathname;
  return (p.replace(/\/+$/, '') || '/').replace(/\/index\.html$/, '') || '/';
}

/** A link target for a page path ("/" or "/demo"). */
export function href(page: string): string {
  return BASE.replace(/\/$/, '') + (page === '/' ? '/' : page);
}

/** A two-page router on the History API: "/" and "/demo". */
export function usePath(): string {
  const [path, setPath] = useState(pagePath(window.location.pathname, BASE));
  useEffect(() => {
    const on = () => setPath(pagePath(window.location.pathname, BASE));
    window.addEventListener('popstate', on);
    return () => window.removeEventListener('popstate', on);
  }, []);
  return path;
}

export function navigate(page: string) {
  const to = href(page);
  if (to === window.location.pathname) return;
  window.history.pushState({}, '', to);
  window.dispatchEvent(new PopStateEvent('popstate'));
  window.scrollTo(0, 0);
}
