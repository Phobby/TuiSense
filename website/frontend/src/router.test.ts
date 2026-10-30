import { describe, expect, it } from 'vitest';
import { pagePath } from './router';

describe('pagePath', () => {
  it('works at the site root', () => {
    expect(pagePath('/', '/')).toBe('/');
    expect(pagePath('/demo', '/')).toBe('/demo');
    expect(pagePath('/demo/', '/')).toBe('/demo');
  });
  it('works under a GitHub Pages project path', () => {
    expect(pagePath('/TuiSense/', '/TuiSense/')).toBe('/');
    expect(pagePath('/TuiSense', '/TuiSense/')).toBe('/');
    expect(pagePath('/TuiSense/demo/', '/TuiSense/')).toBe('/demo');
    expect(pagePath('/TuiSense/demo/index.html', '/TuiSense/')).toBe('/demo');
    expect(pagePath('/TuiSense/index.html', '/TuiSense/')).toBe('/');
  });
});
