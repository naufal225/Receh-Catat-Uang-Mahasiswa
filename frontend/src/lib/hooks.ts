import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError } from './api';

/* ---------- Stale-while-revalidate cache ---------- */
// The last response per endpoint, kept in memory for this tab only (never in
// storage: it is someone's finances). Returning to a page or a period shows it
// instantly while a fresh copy loads, so the server is still the truth.
const MAX_ENTRIES = 60;
const responses = new Map<string, unknown>();

/** Pages add "v=<n>" to refetch after a change; that is not a different view. */
function cacheKey(path: string): string {
  return path.replace(/([?&])v=\d+(&|$)/, (_, lead: string, tail: string) => (tail ? lead : '')).replace(/[?&]$/, '');
}

function remember(key: string, value: unknown) {
  responses.delete(key);
  responses.set(key, value);
  if (responses.size > MAX_ENTRIES) responses.delete(responses.keys().next().value as string);
}

/** Forget everything, e.g. on sign-out, so the next account never sees it. */
export function clearFetchCache() {
  responses.clear();
}

/**
 * Loads a GET endpoint. A cached copy, if any, shows at once while the request
 * runs; otherwise the previous data stays on screen (dimmed by the caller)
 * instead of flashing a skeleton.
 */
export function useFetch<T>(path: string | null) {
  const [data, setData] = useState<T | null>(() => (path ? (responses.get(cacheKey(path)) as T | undefined) ?? null : null));
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(true);
  const [version, setVersion] = useState(0);
  const latest = useRef(0);

  useEffect(() => {
    if (!path) return;
    const id = ++latest.current;
    const key = cacheKey(path);
    const cached = responses.get(key) as T | undefined;
    if (cached !== undefined) setData(cached);
    setLoading(true);
    api
      .get<T>(path)
      .then((d) => {
        remember(key, d);
        if (id === latest.current) {
          setData(d);
          setError(null);
        }
      })
      .catch((e: ApiError) => id === latest.current && setError(e))
      .finally(() => id === latest.current && setLoading(false));
  }, [path, version]);

  const reload = useCallback(() => setVersion((v) => v + 1), []);
  return { data, error, loading, reload, setData };
}

export function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

/** A value remembered per browser. Storage can be unavailable, so every access is guarded. */
export function useStoredState<T extends string>(key: string, fallback: T): [T, (v: T) => void] {
  const [v, setV] = useState<T>(() => {
    try {
      return (localStorage.getItem(key) as T) || fallback;
    } catch {
      return fallback;
    }
  });
  const set = useCallback(
    (next: T) => {
      setV(next);
      try {
        localStorage.setItem(key, next);
      } catch {
        /* ignore */
      }
    },
    [key],
  );
  return [v, set];
}
