import { lazy, Suspense } from 'react';
import { Landing } from './pages/Landing';
import { usePath } from './router';

// The terminal (xterm.js) loads only on the demo page.
const DemoPage = lazy(() => import('./pages/DemoPage').then((m) => ({ default: m.DemoPage })));

export function App() {
  const path = usePath();
  if (path.replace(/\/$/, '') === '/demo') {
    return (
      <Suspense fallback={<div className="demo" />}>
        <DemoPage />
      </Suspense>
    );
  }
  return <Landing />;
}
