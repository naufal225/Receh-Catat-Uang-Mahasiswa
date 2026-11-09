// Two columns, as specified: interactive charts on the left, the latest
// transactions (styled as a receipt) on the right.
import { useEffect, useState } from 'react';
import { BarChart, PieChart } from '../components/charts';
import { Icon } from '../components/Icon';
import { TxRow } from '../components/TxRow';
import { BudgetBar } from '../components/BudgetBar';
import { Empty, ErrorNote, Link, Skeleton } from '../components/ui';
import { api, qs } from '../lib/api';
import { useAuth } from '../lib/auth';
import { useFinance } from '../lib/finance';
import { formatDate, formatRp, frequencyLabel, walletTypeLabel } from '../lib/format';
import { useFetch, useStoredState } from '../lib/hooks';
import { navigate } from '../lib/router';
import type { Bucket, Dashboard as DashboardData, SliceSet, TxType, Unit } from '../lib/types';

const units: { value: Unit; label: string }[] = [
  { value: 'day', label: 'Harian' },
  { value: 'week', label: 'Mingguan' },
  { value: 'month', label: 'Bulanan' },
  { value: 'year', label: 'Tahunan' },
];

function greeting() {
  const h = new Date().getHours();
  if (h < 11) return 'Selamat pagi';
  if (h < 15) return 'Selamat siang';
  if (h < 19) return 'Selamat sore';
  return 'Selamat malam';
}

export default function Dashboard() {
  const { user } = useAuth();
  const { version, wallets, openTx, readOnly, loaded } = useFinance();
  const [unit, setUnit] = useStoredState<Unit>('receh-unit', 'day');
  const [offset, setOffset] = useState(0);
  const [walletId, setWalletId] = useState(0);
  const [focus, setFocus] = useState<Bucket | null>(null);
  const [pieType, setPieType] = useState<TxType>('expense');
  const [activeSlice, setActiveSlice] = useState<number | null>(null);
  const [showTable, setShowTable] = useState(false);

  const { data, error, loading, reload } = useFetch<DashboardData>(`/dashboard${qs({ unit, offset, walletId })}&v=${version}`);

  // A clicked bar narrows the pie to that period.
  const [focusSlices, setFocusSlices] = useState<SliceSet | null>(null);
  useEffect(() => {
    setFocusSlices(null);
    if (!focus) return;
    let live = true;
    api.get<SliceSet>(`/dashboard/slices${qs({ from: focus.from, to: focus.to, walletId })}`).then((s) => live && setFocusSlices(s));
    return () => {
      live = false;
    };
  }, [focus, walletId, version]);

  // Changing the window clears a focus that no longer exists in it.
  useEffect(() => setFocus(null), [unit, offset, walletId]);

  if (!data && error) return <ErrorNote message={error.message} onRetry={reload} />;

  const slicesSrc = focus && focusSlices ? focusSlices : data;
  const slices = slicesSrc ? (pieType === 'expense' ? slicesSrc.expenseSlices : slicesSrc.incomeSlices) : [];
  const sliceTotal = slices.reduce((s, c) => s + c.total, 0);
  const net = data ? data.totals.income - data.totals.expense : 0;
  const noWallets = loaded && wallets.length === 0;

  return (
    <>
      <div className="page-head reveal">
        <div>
          <h1>{greeting()}, {user?.name.split(' ')[0]}</h1>
          <p>Ini gambaran uangmu. Klik batang grafik untuk melihat rincian periode itu.</p>
        </div>
        {!readOnly && !noWallets && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {wallets.length >= 2 && (
              <button className="btn btn-ghost" onClick={() => openTx({ type: 'transfer' })}>
                <Icon name="transfer" size={18} /> Transfer
              </button>
            )}
            <button className="btn btn-ghost" onClick={() => openTx({ type: 'income' })}>
              <Icon name="down" size={18} /> Pemasukan
            </button>
            <button className="btn btn-primary" onClick={() => openTx({ type: 'expense' })}>
              <Icon name="up" size={18} /> Pengeluaran
            </button>
          </div>
        )}
      </div>

      {noWallets && (
        <div className="card card-pad reveal" style={{ marginBottom: 24 }}>
          <Empty icon="wallet" title="Mulai dengan menambahkan dompet"
            action={!readOnly && <Link to="/app/dompet?baru=1" className="btn btn-primary"><Icon name="plus" size={18} /> Tambah dompet pertama</Link>}>
            Masukkan tempat uangmu disimpan: rekening BCA, GoPay, DANA, atau uang tunai beserta saldonya sekarang.
          </Empty>
        </div>
      )}

      <div className="dash-grid">
        {/* ===== Column 1: interactive charts ===== */}
        <section aria-label="Grafik keuangan">
          <div className="hero-balance reveal reveal-2">
            <span className="label">{walletId ? `Saldo ${wallets.find((w) => w.id === walletId)?.name ?? ''}` : 'Total saldo semua dompet'}</span>
            <div className="hero-amount">{data ? formatRp(data.totalBalance) : <Skeleton h={44} w={220} />}</div>
            <div className="hero-stats">
              <div>
                <span><Icon name="down" size={14} /> Pemasukan</span>
                <strong>{data ? formatRp(data.totals.income) : '…'}</strong>
              </div>
              <div>
                <span><Icon name="up" size={14} /> Pengeluaran</span>
                <strong>{data ? formatRp(data.totals.expense) : '…'}</strong>
              </div>
              <div>
                <span>Selisih periode ini</span>
                <strong>{data ? formatRp(net, true) : '…'}</strong>
              </div>
            </div>
          </div>

          {/* One filter row scopes every chart below it. */}
          <div className="filter-bar reveal reveal-3">
            <div className="segmented" role="group" aria-label="Satuan waktu">
              {units.map((u) => (
                <button key={u.value} type="button" aria-pressed={unit === u.value} onClick={() => { setUnit(u.value); setOffset(0); }}>
                  {u.label}
                </button>
              ))}
            </div>
            <div className="period-nav">
              <button className="icon-btn" onClick={() => setOffset((o) => o + 1)} aria-label="Periode sebelumnya"><Icon name="left" size={18} /></button>
              <strong aria-live="polite">{data?.title ?? '…'}</strong>
              <button className="icon-btn" onClick={() => setOffset((o) => Math.max(0, o - 1))} disabled={!data?.canGoForward} aria-label="Periode berikutnya"><Icon name="right" size={18} /></button>
            </div>
            <select className="select" value={walletId} onChange={(e) => setWalletId(Number(e.target.value))} aria-label="Filter dompet">
              <option value={0}>Semua dompet</option>
              {wallets.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
            </select>
          </div>

          <div className={'charts reveal reveal-4' + (loading && data ? ' dim' : '')}>
            <div className="card card-pad">
              <div className="card-head">
                <h2>Arus uang</h2>
                <div className="legend" aria-hidden="true">
                  <span><i style={{ background: 'var(--viz-income)' }} />Pemasukan</span>
                  <span><i style={{ background: 'var(--viz-expense)' }} />Pengeluaran</span>
                </div>
              </div>
              {data ? (
                <BarChart buckets={data.buckets} selected={focus?.key ?? null} onSelect={setFocus} />
              ) : (
                <Skeleton h={250} />
              )}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, marginTop: 10 }}>
                <button className="btn btn-quiet btn-sm data-table-toggle" onClick={() => setShowTable((s) => !s)} aria-expanded={showTable}>
                  <Icon name="table" size={16} /> {showTable ? 'Sembunyikan tabel' : 'Lihat sebagai tabel'}
                </button>
                {focus && (
                  <span className="focus-pill">
                    Fokus: {focus.label}
                    <button className="icon-btn" style={{ width: 26, height: 26 }} onClick={() => setFocus(null)} aria-label="Hapus fokus"><Icon name="close" size={14} /></button>
                  </span>
                )}
              </div>
              {showTable && data && (
                <div className="table-scroll" style={{ marginTop: 8 }}>
                  <table className="table">
                    <thead><tr><th>Periode</th><th style={{ textAlign: 'right' }}>Pemasukan</th><th style={{ textAlign: 'right' }}>Pengeluaran</th></tr></thead>
                    <tbody>
                      {data.buckets.map((b) => (
                        <tr key={b.key}>
                          <td>{b.label}</td>
                          <td className="num" style={{ textAlign: 'right' }}>{formatRp(b.income)}</td>
                          <td className="num" style={{ textAlign: 'right' }}>{formatRp(b.expense)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            <div className="card card-pad">
              <div className="card-head">
                <h2>Per kategori</h2>
                <div className="segmented" role="group" aria-label="Jenis kategori">
                  <button type="button" aria-pressed={pieType === 'expense'} onClick={() => setPieType('expense')}>Keluar</button>
                  <button type="button" aria-pressed={pieType === 'income'} onClick={() => setPieType('income')}>Masuk</button>
                </div>
              </div>
              <p className="muted" style={{ fontSize: 13, marginTop: -8, marginBottom: 10 }}>
                {focus ? `Periode ${focus.label}` : data?.title}
              </p>
              {!data ? (
                <Skeleton h={220} />
              ) : sliceTotal === 0 ? (
                <Empty icon="pie" title="Belum ada data">
                  {pieType === 'expense' ? 'Tidak ada pengeluaran' : 'Tidak ada pemasukan'} pada periode ini.
                </Empty>
              ) : (
                <div className="pie-wrap">
                  <PieChart slices={slices} total={sliceTotal} active={activeSlice} onActive={setActiveSlice} centerLabel={pieType === 'expense' ? 'Total keluar' : 'Total masuk'} />
                  {/* The table is the legend and the accessible twin of the pie. Click to drill into history. */}
                  <table className="cat-table">
                    <tbody>
                      {slices.map((s) => (
                        <tr key={s.categoryId} className={activeSlice === s.categoryId ? 'active' : ''}
                          onMouseEnter={() => setActiveSlice(s.categoryId)} onMouseLeave={() => setActiveSlice(null)}
                          onClick={() => navigate(`/app/transaksi${qs({ categoryId: s.categoryId, from: focus?.from ?? data.from, to: focus?.to ?? data.to, walletId })}`)}
                          title="Lihat transaksinya">
                          <td><span className={'dot swatch-' + s.color} />{s.name}</td>
                          <td className="num">{Math.round((s.total / sliceTotal) * 100)}%</td>
                          <td className="num">{formatRp(s.total)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>

          {data && data.wallets.length > 0 && (
            <div style={{ marginTop: 24 }}>
              <div className="card-head">
                <h2 style={{ fontSize: 17, fontFamily: 'var(--font-sans)', fontWeight: 750 }}>Dompetmu</h2>
                <Link to="/app/dompet" className="btn btn-quiet btn-sm">Kelola</Link>
              </div>
              <div className="wallet-strip">
                {data.wallets.map((w) => (
                  <button key={w.id} type="button" className={'wallet-card wallet-bg-' + w.color} style={{ cursor: 'pointer', outlineOffset: 3, boxShadow: walletId === w.id ? '0 0 0 3px var(--ink)' : undefined }}
                    onClick={() => setWalletId(walletId === w.id ? 0 : w.id)} aria-pressed={walletId === w.id} title="Klik untuk memfilter dasbor">
                    <div className="wc-type">{walletTypeLabel[w.type]}</div>
                    <div className="wc-name">{w.name}</div>
                    <div className="wc-balance">{formatRp(w.balance)}</div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </section>

        {/* ===== Column 2: latest transactions ===== */}
        <aside className="dash-side reveal reveal-3" aria-label="Transaksi terbaru">
          <div className="receipt">
            <div className="receipt-head">
              <div className="mono">Struk terbaru</div>
              <h2 style={{ fontSize: 19, marginTop: 4, fontFamily: 'var(--font-sans)', fontWeight: 800 }}>Transaksi terakhir</h2>
            </div>
            <div className="receipt-list">
              {!data ? (
                <div style={{ display: 'grid', gap: 14, paddingTop: 14 }}>{[0, 1, 2, 3].map((i) => <Skeleton key={i} h={38} />)}</div>
              ) : data.latest.length === 0 ? (
                <Empty icon="list" title="Belum ada transaksi">
                  {readOnly ? 'Tidak ada data untuk ditampilkan.' : 'Tekan "Catat transaksi" atau tombol N untuk mencatat yang pertama.'}
                </Empty>
              ) : (
                data.latest.map((t) => <TxRow key={t.id} t={t} />)
              )}
            </div>
            <div className="receipt-foot">
              <span>{data?.latest.length ?? 0} item</span>
              <Link to="/app/transaksi" style={{ fontWeight: 600 }}>Lihat semua →</Link>
            </div>
          </div>

          {data && !readOnly && (
            <div className="card card-pad" style={{ marginTop: 6 }}>
              <div className="card-head" style={{ marginBottom: 10 }}>
                <h3>Anggaran bulan ini</h3>
                <Link to="/app/anggaran" className="btn btn-quiet btn-sm">{data.budgets.length ? 'Semua' : 'Buat'}</Link>
              </div>
              {data.budgets.length === 0 ? (
                <p className="muted" style={{ fontSize: 13.5 }}>Tetapkan batas belanja per kategori, misalnya Makan Rp 1.200.000, supaya tahu kapan harus mengerem.</p>
              ) : (
                <div style={{ display: 'grid', gap: 14 }}>
                  {[...data.budgets].sort((a, b) => b.percent - a.percent).slice(0, 4).map((b) => <BudgetBar key={b.id} b={b} compact />)}
                </div>
              )}
            </div>
          )}

          {data && data.upcomingRoutines.length > 0 && (
            <div className="card card-pad" style={{ marginTop: 6 }}>
              <div className="card-head" style={{ marginBottom: 8 }}>
                <h3>Jadwal rutin berikutnya</h3>
                <Link to="/app/rutin" className="btn btn-quiet btn-sm">Semua</Link>
              </div>
              {data.upcomingRoutines.map((r) => (
                <div key={r.id} className="tx-row">
                  <div className={'tx-icon swatch-' + r.categoryColor}><Icon name="repeat" size={18} /></div>
                  <div className="tx-main">
                    <div className="tx-name">{r.name}</div>
                    <div className="tx-meta">{formatDate(r.nextDue)} · {frequencyLabel[r.frequency]}</div>
                    {r.lastError && <div className="tx-meta" style={{ color: 'var(--warn)' }}><Icon name="alert" size={12} /> Menunggu saldo cukup</div>}
                  </div>
                  <div className={'tx-amount ' + (r.type === 'income' ? 'income-text' : 'expense-text')}>{formatRp(r.type === 'income' ? r.amount : -r.amount, true)}</div>
                </div>
              ))}
            </div>
          )}
        </aside>
      </div>
    </>
  );
}
