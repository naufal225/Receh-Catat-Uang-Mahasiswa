// Admin: every payment, plus a checklist of which gateway settings are filled in.
import { useState } from 'react';
import { Icon } from '../components/Icon';
import { Empty, ErrorNote, Skeleton, useToast } from '../components/ui';
import { api, ApiError, qs } from '../lib/api';
import { formatDateTime, formatRp } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { IntegrationStatus, Payment, PaymentStatus } from '../lib/types';
import { paymentStatusLabel } from './Subscription';

export default function AdminPayments() {
  const toast = useToast();
  const [status, setStatus] = useState<PaymentStatus | ''>('');
  const [page, setPage] = useState(1);
  const info = useFetch<IntegrationStatus>('/admin/billing/status');
  const list = useFetch<{ items: Payment[]; total: number }>(`/admin/payments${qs({ status, page })}`);
  const totalPages = list.data ? Math.max(1, Math.ceil(list.data.total / 25)) : 1;

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      toast('Disalin');
    } catch {
      toast('Tidak bisa menyalin otomatis. Salin manual.', 'error');
    }
  }

  async function sync(p: Payment) {
    try {
      await api.post(`/admin/payments/${p.id}/sync`);
      toast('Status diperiksa ke penyedia');
      list.reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const i = info.data;
  return (
    <>
      <div className="page-head">
        <div>
          <h1>Pembayaran</h1>
          <p>Semua pembayaran langganan dan status pengaturan penyedia pembayaran.</p>
        </div>
        <button className="btn btn-ghost" onClick={() => { info.reload(); list.reload(); }}><Icon name="repeat" size={18} /> Muat ulang</button>
      </div>

      {i && !i.enabled && (
        <div className="alert alert-info" style={{ marginBottom: 16 }}>
          Server ini berjalan sebagai <strong>edisi pribadi</strong> (<code>BILLING_ENABLED=false</code>): tidak ada pembayaran.
        </div>
      )}

      {i?.enabled && (
        <div className="card card-pad" style={{ marginBottom: 16, display: 'grid', gap: 10 }}>
          <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
            <strong>Penyedia: {i.provider}</strong>
            <span className={'badge ' + (i.mode === 'production' ? 'badge-income' : 'badge-warn')}>{i.mode}</span>
          </div>
          {i.keys.length > 0 && (
            <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: 6 }}>
              {i.keys.map((k) => (
                <li key={k.name} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <Icon name={k.set ? 'check' : 'close'} size={16} />
                  <code>{k.name}</code>
                  <span className="muted">{k.set ? 'terisi' : 'belum diisi (di backend/.env)'}</span>
                </li>
              ))}
            </ul>
          )}
          {i.webhookUrl && (
            <div>
              <p className="muted" style={{ fontSize: 13.5 }}>Tempel URL ini di dashboard penyedia sebagai URL notifikasi (webhook):</p>
              <button className="promo-code" onClick={() => copy(i.webhookUrl)} title="Salin"><span className="mono">{i.webhookUrl}</span> <Icon name="copy" size={14} /></button>
            </div>
          )}
          {i.provider === 'mock' && <p className="muted" style={{ fontSize: 13.5 }}>Gateway tiruan: tidak ada uang sungguhan. Ganti <code>BILLING_PROVIDER</code> ke <code>midtrans</code> atau <code>xendit</code> bila kunci sudah siap.</p>}
        </div>
      )}

      <div style={{ marginBottom: 12 }}>
        <select className="select" style={{ width: 'auto' }} value={status} aria-label="Status" onChange={(e) => { setStatus(e.target.value as PaymentStatus | ''); setPage(1); }}>
          <option value="">Semua status</option>
          {(Object.keys(paymentStatusLabel) as PaymentStatus[]).map((s) => <option key={s} value={s}>{paymentStatusLabel[s].label}</option>)}
        </select>
      </div>
      {list.error && !list.data && <ErrorNote message={list.error.message} onRetry={list.reload} />}

      <div className={'card' + (list.loading && list.data ? ' dim' : '')} style={{ overflow: 'hidden' }}>
        {!list.data ? (
          <div style={{ padding: 18, display: 'grid', gap: 12 }}>{[0, 1].map((n) => <Skeleton key={n} h={42} />)}</div>
        ) : list.data.items.length === 0 ? (
          <Empty icon="sparkle" title="Belum ada pembayaran">Pembayaran muncul di sini setelah ada yang berlangganan.</Empty>
        ) : (
          <div className="table-scroll">
            <table className="table">
              <thead><tr><th>Waktu</th><th>Workspace / pembayar</th><th>Paket</th><th className="num">Jumlah</th><th>Status</th><th style={{ textAlign: 'right' }}>Aksi</th></tr></thead>
              <tbody>
                {list.data.items.map((p) => (
                  <tr key={p.id}>
                    <td>{formatDateTime(p.paidAt ?? p.createdAt)}</td>
                    <td>{p.orgName || '—'}<span className="sub">{p.payerEmail}</span></td>
                    <td>{p.priceId}<span className="sub">{p.provider}</span></td>
                    <td className="num">{formatRp(p.amount)}</td>
                    <td><span className={'badge ' + paymentStatusLabel[p.status].cls}>{paymentStatusLabel[p.status].label}</span></td>
                    <td style={{ textAlign: 'right' }}>
                      {p.status === 'pending' && <button className="btn btn-ghost btn-sm" onClick={() => sync(p)}>Periksa</button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {totalPages > 1 && (
        <div style={{ display: 'flex', gap: 10, justifyContent: 'center', marginTop: 14, alignItems: 'center' }}>
          <button className="btn btn-ghost btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button>
          <span className="muted">Halaman {page} dari {totalPages}</span>
          <button className="btn btn-ghost btn-sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Berikutnya</button>
        </div>
      )}
    </>
  );
}
