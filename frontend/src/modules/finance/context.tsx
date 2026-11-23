// Finance module state: wallets and categories (used by nearly every form) and
// the global "Catat transaksi" modal. Version, plan and read-only come from
// the core app context and are passed through for convenience.
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { api } from '../../lib/api';
import { useAuth } from '../../lib/auth';
import { useApp } from '../../lib/app';
import { useOrg } from '../../lib/org';
import type { Membership } from '../../lib/types';
import type { Category, Transaction, TxType, Wallet } from './types';

/** What the global "Catat" modal shows: an income/expense form or a transfer form. */
export type EntryMode = TxType | 'transfer';

export interface TxModalState {
  open: boolean;
  mode: EntryMode;
  /** Editing an income or expense. */
  edit: Transaction | null;
  /** Editing a transfer (opened from any of its rows). */
  transferId: number | null;
  /** New transfer from a given wallet. */
  fromWalletId: number | null;
}

export interface OpenTxOptions {
  edit?: Transaction;
  type?: EntryMode;
  fromWalletId?: number;
}

interface FinanceState {
  wallets: Wallet[];
  categories: Category[];
  loaded: boolean;
  version: number;
  /** Refetch wallets/categories and tell pages their data changed. */
  changed: () => void;
  txModal: TxModalState;
  openTx: (opts?: OpenTxOptions) => void;
  closeTx: () => void;
  readOnly: boolean;
  /** Plan, limits and usage; null until loaded. */
  membership: Membership | null;
}

const Ctx = createContext<FinanceState>(null as unknown as FinanceState);
export const useFinance = () => useContext(Ctx);

export function FinanceProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const [wallets, setWallets] = useState<Wallet[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loaded, setLoaded] = useState(false);
  const app = useApp();
  const orgId = useOrg().active?.id ?? 0;
  const [txModal, setTxModal] = useState<TxModalState>({ open: false, mode: 'expense', edit: null, transferId: null, fromWalletId: null });

  const load = useCallback(async () => {
    const [w, c] = await Promise.all([api.get<Wallet[]>('/wallets'), api.get<Category[]>('/categories')]);
    setWallets(w);
    setCategories(c);
    setLoaded(true);
  }, []);

  useEffect(() => {
    if (user?.role === 'user') load().catch(() => setLoaded(true));
  }, [user?.id, user?.role, orgId, load]);

  const appChanged = app.changed;
  const changed = useCallback(() => {
    appChanged();
    load().catch(() => {});
  }, [appChanged, load]);

  const openTx = useCallback((opts: OpenTxOptions = {}) => {
    const e = opts.edit;
    // Any row of a transfer (either leg or its fee) edits the whole transfer.
    if (e?.transferId) {
      setTxModal({ open: true, mode: 'transfer', edit: null, transferId: e.transferId, fromWalletId: null });
      return;
    }
    const mode: EntryMode = e ? (e.type === 'income' ? 'income' : 'expense') : (opts.type ?? 'expense');
    setTxModal({ open: true, mode, edit: e ?? null, transferId: null, fromWalletId: opts.fromWalletId ?? null });
  }, []);
  const closeTx = useCallback(() => setTxModal((m) => ({ ...m, open: false })), []);

  return (
    <Ctx.Provider value={{ wallets, categories, loaded, version: app.version, changed, txModal, openTx, closeTx, readOnly: app.readOnly, membership: app.membership }}>
      {children}
    </Ctx.Provider>
  );
}
