// Thin fetch wrapper. The session lives in an HttpOnly cookie, so no token is
// ever handled by JavaScript.

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public field?: string,
  ) {
    super(message);
  }
}

type Listener = (err: ApiError) => void;
const listeners = new Set<Listener>();

/** Lets the auth layer react globally to an expired session or a freeze. */
export function onApiError(fn: Listener): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

// The workspace requests act in (X-Org-Id). The server checks membership on
// every request, so this is a choice, never a permission. 0 = personal workspace.
let activeOrg = 0;
export function setActiveOrg(id: number) {
  activeOrg = id;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response;
  const headers: Record<string, string> = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (activeOrg) headers['X-Org-Id'] = String(activeOrg);
  try {
    res = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'NETWORK', 'Tidak bisa terhubung ke server. Periksa koneksi Anda.');
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    const err = new ApiError(
      res.status,
      data?.code ?? 'UNKNOWN',
      data?.message ?? 'Terjadi kesalahan. Coba lagi.',
      data?.field || undefined,
    );
    listeners.forEach((fn) => fn(err));
    throw err;
  }
  return data as T;
}

export function qs(params: Record<string, string | number | undefined | null>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '' && v !== 0) q.set(k, String(v));
  }
  const s = q.toString();
  return s ? '?' + s : '';
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  del: <T = void>(path: string) => request<T>('DELETE', path),
};
