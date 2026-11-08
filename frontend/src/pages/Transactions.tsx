// Full history with filters that live in the URL, so a filtered view can be
// bookmarked and the dashboard can deep-link into it.
import { useMemo, useState } from 'react';
import { Icon } from '../components/Icon';
import { TxRow } from '../components/TxRow';
import { Empty, ErrorNote, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError, qs } from '../lib/api';
import { useFinance } from '../lib/finance';
import { formatDayHeading, formatRp, shiftISO, todayISO } from '../lib/format';
import { useDebounced, useFetch } from '../lib/hooks';
import { navigate, useLocation } from '../lib/router';
import type { Page, Transaction } from '../lib/types';

export default function Transactions() {
  const { query } = useLocation();
  const { wallets, categories, version, openTx, changed, readOnly } = useFinance();
  const toast = useToast();
  const confirm = useConfirm();
  const [search, setSearch] = useState(query.get('search') ?? '');
  const debounced = useDebounced(search);

  const f = {
    search: debounced,
    type: query.get('type') ?? '',
    walletId: Number(query.get('walletId') ?? 0),
    categoryId: Number(query.get('categoryId') ?? 0),
    from: query.get('from') ?? '',
    to: query.get('to') ?? '',
    page: Number(query.get('page') ?? 1),
  };
  const set = (patch: Partial<typeof f>) => {
    const next = { ...f, search, page: 1, ...patch };
    navigate('/app/transaksi' + qs({ ...next, page: next.page > 1 ? next.page : undefined }), { replace: true });
  };

  const { data, error, loading, reload } = useFetch<Page<Transaction>>(`/transactions${qs({ ...f, pageSize: 25 })}&v=${version}`);
  const hasFilter = !!(f.search || f.type || f.walletId || f.categoryId || f.from || f.to);
  const cats = categories.filter((c) => !f.type || c.type === f.type);
  const isTransferFilter = f.type === 'transfer';

  // Group the page by date for a calm, scannable list.
  const groups = useMemo(() => {
    const m = new Map<string, Transaction[]>();
    data?.items.forEach((t) => m.set(t.occurredOn, [...(m.get(t.occurredOn) ?? []), t]));
    return [...m.entries()];
  }, [data]);

  async function remove(t: Transaction) {
    if (t.transferId) return removeTransfer(t);
    const ok = await confirm({
      title: 'Hapus transaksi?',
      body: <>“{t.name}” sebesar <strong>{formatRp(t.amount)}</strong> akan dihapus dan saldo {t.walletName} dikembalikan.</>,
      confirmLabel: 'Hapus',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/transactions/${t.id}`);
      toast('Transaksi dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  // Any row of a transfer removes the whole transfer, so balances stay whole.
  async function removeTransfer(t: Transaction) {
    const ok = await confirm({
      title: 'Hapus transfer?',
      body: <>Transfer ini dihapus seluruhnya, termasuk biaya adminnya, dan saldo kedua dompet dikembalikan.</>,
      confirmLabel: 'Hapus transfer',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del(`/transfers/${t.transferId}`);
      toast('Transfer dihapus');
      changed();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const today = todayISO();
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.pageSize)) : 1;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Riwayat transaksi</h1>
          <p>Semua pemasukan, pengeluaran, dan transfer antar dompet. Cari, saring, ubah, atau hapus.</p>
        </div>
        {!readOnly && <button className="btn btn-primary" onClick={() => openTx()}><Icon name="plus" size={18} /> Catat transaksi</button>}
      </div>

      <div className="filters">
        <div style={{ position: 'relative' }}>
          <input className="input" style={{ paddingLeft: 40 }} placeholder="Cari nama kegiatan…" value={search} onChange={(e) => setSearch(e.target.value)} aria-label="Cari transaksi"
            onBlur={() => set({ search })} onKeyDown={(e) => e.key === 'Enter' && set({ search })} />
          <span style={{ position: 'absolute', left: 13, top: 12, color: 'var(--muted)' }}><Icon name="search" size={18} /></span>
        </div>
        <select className="select" value={f.type} onChange={(e) => set({ type: e.target.value, categoryId: 0 })} aria-label="Jenis">
          <option value="">Semua jenis</option>
          <option value="expense">Pengeluaran</option>
          <option value="income">Pemasukan</option>
          <option value="transfer">Transfer</option>
        </select>
        <select className="select" value={f.walletId} onChange={(e) => set({ walletId: Number(e.target.value) })} aria-label="Dompet">
          <option value={0}>Semua dompet</option>
          {wallets.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
        </select>
        <select className="select" value={f.categoryId} onChange={(e) => set({ categoryId: Number(e.target.value) })} aria-label="Kategori" disabled={isTransferFilter}>
          <option value={0}>Semua kategori</option>
          {cats.map((c) => <option key={c.id} value={c.id}>{c.name} ({c.type === 'expense' ? 'keluar' : 'masuk'})</option>)}
        </select>
        {hasFilter && <button className="btn btn-quiet" onClick={() => { setSearch(''); navigate('/app/transaksi', { replace: true }); }}>Reset</button>}
      </div>
      <div className="filters-dates">
        <span className="label">Rentang tanggal</span>
        {[
          { label: '7 hari', from: shiftISO(today, -6) },
          { label: '30 hari', from: shiftISO(today, -29) },
          { label: 'Bulan ini', from: today.slice(0, 8) + '01' },
        ].map((p) => (
          <button key={p.label} className="chip" aria-pressed={f.from === p.from && f.to === today} onClick={() => set({ from: p.from, to: today })}>{p.label}</button>
        ))}
        <input type="date" className="input" value={f.from} max={f.to || undefined} onChange={(e) => set({ from: e.target.value })} aria-label="Dari tanggal" />
        <span className="muted">s.d.</span>
        <input type="date" className="input" value={f.to} min={f.from || undefined} onChange={(e) => set({ to: e.target.value })} aria-label="Sampai tanggal" />
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}

      <div className={'card' + (loading && data ? ' dim' : '')} style={{ overflow: 'hidden' }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 14 }}>{[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} h={40} />)}</div>
        ) : data.items.length === 0 ? (
          <Empty icon="list" title={hasFilter ? 'Tidak ada yang cocok' : 'Belum ada transaksi'}>
            {hasFilter ? 'Coba ubah atau reset filternya.' : 'Transaksi yang kamu catat akan muncul di sini.'}
          </Empty>
        ) : (
          <>
            {groups.map(([day, items]) => {
              // Transfers move money between own wallets, so they do not change the day's net.
              const net = items.reduce((s, t) => s + (t.type === 'income' ? t.amount : t.type === 'expense' ? -t.amount : 0), 0);
              return (
                <div key={day} className="day-group">
                  <h3><span>{formatDayHeading(day)}</span><span className="num">{formatRp(net, true)}</span></h3>
                  {items.map((t) => (
                    <TxRow key={t.id} t={t} showDate={false}
                      actions={!readOnly && (
                        <div className="row-actions">
                          <button className="icon-btn" onClick={() => openTx({ edit: t })} aria-label={`Ubah ${t.name}`}><Icon name="edit" size={18} /></button>
                          <button className="icon-btn" onClick={() => remove(t)} aria-label={`Hapus ${t.name}`}><Icon name="trash" size={18} /></button>
                        </div>
                      )} />
                  ))}
                </div>
              );
            })}
            <div className="pager">
              <span>{data.total} transaksi · halaman {data.page} dari {totalPages}</span>
              <div style={{ display: 'flex', gap: 6 }}>
                <button className="btn btn-ghost btn-sm" disabled={data.page <= 1} onClick={() => set({ page: data.page - 1 })}><Icon name="left" size={16} /> Sebelumnya</button>
                <button className="btn btn-ghost btn-sm" disabled={data.page >= totalPages} onClick={() => set({ page: data.page + 1 })}>Berikutnya <Icon name="right" size={16} /></button>
              </div>
            </div>
          </>
        )}
      </div>
    </>
  );
}
