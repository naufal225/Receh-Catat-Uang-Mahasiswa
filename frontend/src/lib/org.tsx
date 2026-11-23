// The workspaces the signed-in user belongs to and the one they act in. Module
// data follows the active workspace: switching clears cached responses and
// tells every page to refetch (via AppProvider's version).
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { brand } from '../brand';
import { api, setActiveOrg } from './api';
import { useAuth } from './auth';
import { clearFetchCache } from './hooks';
import type { OrgMembership } from './types';

interface OrgState {
  orgs: OrgMembership[];
  /** The workspace requests act in; null for admins (they have none). */
  active: OrgMembership | null;
  switchTo: (id: number) => void;
  /** Reload the list (after creating, renaming, leaving, deleting). */
  refresh: () => Promise<OrgMembership[]>;
}

const Ctx = createContext<OrgState>(null as unknown as OrgState);
export const useOrg = () => useContext(Ctx);

export function OrgProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [orgs, setOrgs] = useState<OrgMembership[]>([]);
  const [activeId, setActiveId] = useState(0);
  // Which user the list belongs to. Children render only once it matches, so
  // no request is ever sent with a previous account's workspace.
  const [loadedFor, setLoadedFor] = useState<number | null>(null);
  const hasOrgs = user?.role === 'user';
  const storeKey = user ? `${brand.storagePrefix}-org-${user.id}` : '';

  const load = useCallback(async () => {
    const list = await api.get<OrgMembership[]>('/orgs');
    setOrgs(list);
    return list;
  }, []);

  useEffect(() => {
    if (!user) {
      setActiveOrg(0);
      setOrgs([]);
      setActiveId(0);
      setLoadedFor(null);
      return;
    }
    if (!hasOrgs) {
      setActiveOrg(0);
      setLoadedFor(user.id);
      return;
    }
    let off = false;
    load()
      .then((list) => {
        if (off) return;
        let id = 0;
        try {
          id = Number(localStorage.getItem(storeKey)) || 0;
        } catch {
          /* storage unavailable: use the personal workspace */
        }
        if (!list.some((o) => o.id === id)) id = (list.find((o) => o.personal) ?? list[0])?.id ?? 0;
        setActiveOrg(id);
        setActiveId(id);
        setLoadedFor(user.id);
      })
      .catch(() => {
        if (off) return;
        setActiveOrg(0);
        setLoadedFor(user.id);
      });
    return () => {
      off = true;
    };
  }, [user, hasOrgs, storeKey, load]);

  const switchTo = useCallback(
    (id: number) => {
      setActiveOrg(id);
      clearFetchCache();
      setActiveId(id);
      try {
        localStorage.setItem(storeKey, String(id));
      } catch {
        /* remembered for this tab only */
      }
    },
    [storeKey],
  );

  const refresh = useCallback(async () => {
    const list = await load();
    if (!list.some((o) => o.id === activeId)) {
      const fallback = list.find((o) => o.personal) ?? list[0];
      if (fallback) switchTo(fallback.id);
    }
    return list;
  }, [load, activeId, switchTo]);

  const active = orgs.find((o) => o.id === activeId) ?? null;
  if (user && loadedFor !== user.id) return null;
  return <Ctx.Provider value={{ orgs, active, switchTo, refresh }}>{children}</Ctx.Provider>;
}
