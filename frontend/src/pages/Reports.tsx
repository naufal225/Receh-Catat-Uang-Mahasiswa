import { useState } from 'react';
import { Icon } from '../components/Icon';
import { ErrorNote, Skeleton } from '../components/ui';
import { useFinance } from '../lib/finance';
import { formatRp, monthLabel, todayISO } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { MonthlyReport } from '../lib/types';

/** The last 12 months as YYYY-MM, newest first. */
function recentMonths() {
  const out: string[] = [];
  const d = new Date();
  d.setDate(1);
  for (let i = 0; i < 12; i++) {
    out.push(todayISO(d).slice(0, 7));
    d.setMonth(d.getMonth() - 1);
  }
  return out;
}

export default function Reports() {
  const months = recentMonths();
  const [month, setMonth] = useState(months[0]);
  const { version } = useFinance();
  const { data, error, loading, reload } = useFetch<MonthlyReport>(`/reports/monthly?month=${month}&v=${version}`);
  const net = data ? data.totals.income - data.totals.expense : 0;
  const top = data?.expenseSlices.slice(0, 5) ?? [];

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Laporan bulanan</h1>
          <p>Lihat ringkasannya di sini, lalu unduh versi PDF untuk disimpan atau dikirim.</p>
        </div>
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          <select className="select" style={{ width: 'auto' }} value={month} onChange={(e) => setMonth(e.target.value)} aria-label="Pilih bulan">
            {months.map((m) => <option key={m} value={m}>{monthLabel(m)}</option>)}
          </select>
          {/* A plain link: the browser handles the download and the cookie authenticates it. */}
          <a className="btn btn-primary" href={`/api/reports/monthly.pdf?month=${month}`} download>
            <Icon name="download" size={18} /> Unduh PDF
          </a>
        </div>
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}

      <div className={loading && data ? 'dim' : undefined}>
        <div className="stat-row">
          <div className="card stat"><span className="label">Pemasukan</span><strong className="income-text">{data ? formatRp(data.totals.income) : <Skeleton h={26} />}</strong></div>
          <div className="card stat"><span className="label">Pengeluaran</span><strong className="expense-text">{data ? formatRp(data.totals.expense) : <Skeleton h={26} />}</strong></div>
          <div className="card stat"><span className="label">Selisih</span><strong>{data ? formatRp(net, true) : <Skeleton h={26} />}</strong></div>
          <div className="card stat"><span className="label">Jumlah transaksi</span><strong>{data ? data.transactions.length : <Skeleton h={26} />}</strong></div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 20 }}>
          <div className="card card-pad">
            <div className="card-head"><h3>Pengeluaran terbesar</h3></div>
            {top.length === 0 ? <p className="muted">Belum ada pengeluaran di {monthLabel(month)}.</p> : (
              <div style={{ display: 'grid', gap: 14 }}>
                {top.map((s) => {
                  const pct = data!.totals.expense ? (s.total / data!.totals.expense) * 100 : 0;
                  return (
                    <div key={s.categoryId}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 14, marginBottom: 6 }}>
                        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span className={'dot swatch-' + s.color} />{s.name}</span>
                        <span className="num"><span className="muted">{Math.round(pct)}% · </span><strong>{formatRp(s.total)}</strong></span>
                      </div>
                      <div style={{ height: 8, borderRadius: 4, background: 'var(--surface-3)' }}>
                        <div className={'swatch-' + s.color} style={{ width: `${pct}%`, height: '100%', borderRadius: 4 }} />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
          <div className="card" style={{ overflow: 'hidden' }}>
            <div className="card-pad" style={{ paddingBottom: 8 }}><h3 style={{ fontSize: 17 }}>Per dompet</h3></div>
            <div className="table-scroll">
              <table className="table">
                <thead><tr><th>Dompet</th><th style={{ textAlign: 'right' }}>Masuk</th><th style={{ textAlign: 'right' }}>Keluar</th></tr></thead>
                <tbody>
                  {(data?.wallets ?? []).map((w) => (
                    <tr key={w.name}>
                      <td>{w.name}<span className="sub">Saldo kini {formatRp(w.balance)}{w.transferIn - w.transferOut !== 0 && <> · transfer {formatRp(w.transferIn - w.transferOut, true)}</>}</span></td>
                      <td className="num income-text" style={{ textAlign: 'right' }}>{formatRp(w.income)}</td>
                      <td className="num expense-text" style={{ textAlign: 'right' }}>{formatRp(w.expense)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
        <p className="muted" style={{ fontSize: 13.5, marginTop: 16 }}>
          PDF berisi ringkasan, rincian per dompet, diagram per kategori, dan daftar lengkap semua transaksi bulan tersebut.
        </p>
      </div>
    </>
  );
}
