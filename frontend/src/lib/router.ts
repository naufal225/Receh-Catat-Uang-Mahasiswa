// A 30-line router on the History API, so the app needs no routing dependency.
import { useSyncExternalStore } from 'react';

const listeners = new Set<() => void>();

function subscribe(fn: () => void) {
  listeners.add(fn);
  window.addEventListener('popstate', fn);
  return () => {
    listeners.delete(fn);
    window.removeEventListener('popstate', fn);
  };
}

const snapshot = () => window.location.pathname + window.location.search;

export function navigate(to: string, opts: { replace?: boolean } = {}) {
  if (to === snapshot()) return;
  if (opts.replace) window.history.replaceState(null, '', to);
  else window.history.pushState(null, '', to);
  listeners.forEach((fn) => fn());
  if (!opts.replace) window.scrollTo(0, 0);
}

/** Current location, re-rendering on navigation. */
export function useLocation(): { path: string; query: URLSearchParams } {
  const loc = useSyncExternalStore(subscribe, snapshot);
  const [path, search = ''] = loc.split('?');
  return { path, query: new URLSearchParams(search) };
}
