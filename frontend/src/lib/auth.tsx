// Holds the signed-in user and reacts to session/freeze errors from any request.
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { api, onApiError } from './api';
import { clearFetchCache } from './hooks';
import type { User } from './types';

interface AuthState {
  user: User | null;
  ready: boolean;
  setUser: (u: User | null) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

const Ctx = createContext<AuthState>(null as unknown as AuthState);
export const useAuth = () => useContext(Ctx);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);

  // Cached responses belong to one account: drop them whenever it changes,
  // including a sign-out forced by another device or an admin.
  useEffect(() => {
    clearFetchCache();
  }, [user?.id]);

  const refresh = useCallback(async () => {
    try {
      setUser(await api.get<User>('/auth/me'));
    } catch {
      setUser(null);
    }
  }, []);

  useEffect(() => {
    refresh().finally(() => setReady(true));
  }, [refresh]);

  useEffect(
    () =>
      onApiError((err) => {
        // Signed out elsewhere (another device revoked us, or a suspension).
        if (err.status === 401 && err.code === 'UNAUTHENTICATED') setUser(null);
        // An admin froze the account while it was open: show the banner now.
        if (err.code === 'ACCOUNT_FROZEN') refresh();
      }),
    [refresh],
  );

  const logout = useCallback(async () => {
    try {
      await api.post('/auth/logout');
    } finally {
      clearFetchCache();
      setUser(null);
    }
  }, []);

  return <Ctx.Provider value={{ user, ready, setUser, refresh, logout }}>{children}</Ctx.Provider>;
}
