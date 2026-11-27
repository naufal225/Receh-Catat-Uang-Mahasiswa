// A page to open after signing in, kept for this tab only. Used by invitation
// links: a visitor who still has to sign in or register comes back to it.
const KEY = 'pending-next';

export function setNext(path: string) {
  try {
    sessionStorage.setItem(KEY, path);
  } catch {
    /* storage unavailable: the visitor reopens the link */
  }
}

/** Returns the stored path once (internal paths only) and forgets it. */
export function takeNext(): string | null {
  try {
    const v = sessionStorage.getItem(KEY);
    sessionStorage.removeItem(KEY);
    return v && v.startsWith('/') && !v.startsWith('//') ? v : null;
  } catch {
    return null;
  }
}
