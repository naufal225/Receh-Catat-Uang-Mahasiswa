// Core app state shared by every module: the user's plan (limits and usage),
// a data version that pages watch to refetch after any change, and whether
// the account is read-only (frozen).
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from './api';
import { useAuth } from './auth';
import { useOrg } from './org';
import type { Membership } from './types';

interface AppState {
  /** Plan, limits and usage; null until loaded (and for admins). */
  membership: Membership | null;
  /** Bumped by changed(); pages refetch when it moves. */
  version: number;
  /** Tell the app that the user's data changed (refreshes plan usage too). */
  changed: () => void;
  readOnly: boolean;
}

const Ctx = createContext<AppState>(null as unknown as AppState);
export const useApp = () => useContext(Ctx);

export function AppProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const { active } = useOrg();
  const orgId = active?.id ?? 0;
  const [membership, setMembership] = useState<Membership | null>(null);
  const [version, setVersion] = useState(0);

  const loadMembership = useCallback(() => {
    api.get<Membership>('/membership').then(setMembership).catch(() => {});
  }, []);

  // The plan belongs to the workspace: reload it, and tell every page to
  // refetch, whenever the active workspace changes.
  useEffect(() => {
    setMembership(null);
    setVersion((v) => v + 1);
    if (user?.role === 'user') loadMembership();
  }, [user?.id, user?.role, orgId, loadMembership]);

  const changed = useCallback(() => {
    setVersion((v) => v + 1);
    loadMembership();
  }, [loadMembership]);

  return (
    <Ctx.Provider value={{ membership, version, changed, readOnly: user?.status === 'frozen' }}>
      {children}
    </Ctx.Provider>
  );
}

/** A plan limit by key: null when there is no cap (unlimited or billing off). */
export function limitOf(m: Membership | null, key: string): number | null {
  if (!m || !m.billingEnabled) return null;
  const v = m.limits[key];
  return v === undefined || v === -1 ? null : v;
}

/** Whether an on/off plan feature is on (always on with billing off). */
export function hasFeature(m: Membership | null, key: string): boolean {
  if (!m) return false;
  if (!m.billingEnabled) return true;
  return (m.limits[key] ?? 1) !== 0;
}
