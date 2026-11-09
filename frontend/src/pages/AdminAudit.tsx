import { useState } from 'react';
import { Icon } from '../components/Icon';
import { Empty, ErrorNote, Skeleton } from '../components/ui';
import { qs } from '../lib/api';
import { formatDateTime } from '../lib/format';
import { useDebounced, useFetch } from '../lib/hooks';
import type { AuditLog, Page } from '../lib/types';

const events: Record<string, { label: string; cls: string }> = {
  login_success: { label: 'Masuk', cls: 'badge-income' },
  login_failed: { label: 'Gagal masuk', cls: 'badge-danger' },
  login_blocked: { label: 'Masuk diblokir', cls: 'badge-danger' },
  logout: { label: 'Keluar', cls: 'badge-neutral' },
  register: { label: 'Daftar', cls: 'badge-accent' },
  session_revoked: { label: 'Perangkat dikeluarkan', cls: 'badge-warn' },
  password_changed: { label: 'Ganti sandi', cls: 'badge-warn' },
  google_linked: { label: 'Google ditautkan', cls: 'badge-accent' },
  premium_granted: { label: 'Admin: beri Premium', cls: 'badge-accent' },
  premium_revoked: { label: 'Admin: cabut Premium', cls: 'badge-warn' },
  promo_created: { label: 'Admin: buat kode', cls: 'badge-neutral' },
  promo_redeemed: { label: 'Tukar kode', cls: 'badge-income' },
  admin_user_created: { label: 'Admin: buat akun', cls: 'badge-neutral' },
  admin_status_changed: { label: 'Admin: ubah status', cls: 'badge-warn' },
};

export default function AdminAudit() {
  const [search, setSearch] = useState('');
  const [event, setEvent] = useState('');
  const [page, setPage] = useState(1);
  const debounced = useDebounced(search);
  const { data, error, loading, reload } = useFetch<Page<AuditLog>>(`/admin/audit-logs${qs({ search: debounced, event, page, pageSize: 25 })}`);
  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.pageSize)) : 1;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Audit log</h1>
          <p>Setiap aktivitas masuk dan tindakan admin: waktu, lokasi, IP, dan perangkat.</p>
        </div>
        <button className="btn btn-ghost" onClick={reload}><Icon name="repeat" size={18} /> Muat ulang</button>
      </div>
      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', marginBottom: 16 }}>
        <input className="input" style={{ maxWidth: 360 }} placeholder="Cari email, IP, perangkat, atau lokasi…" value={search} onChange={(e) => { setSearch(e.target.value); setPage(1); }} aria-label="Cari audit log" />
        <select className="select" style={{ width: 'auto' }} value={event} onChange={(e) => { setEvent(e.target.value); setPage(1); }} aria-label="Jenis kejadian">
          <option value="">Semua kejadian</option>
          {Object.entries(events).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
        </select>
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}
      <div className={'card' + (loading && data ? ' dim' : '')} style={{ overflow: 'hidden' }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 12 }}>{[0, 1, 2, 3].map((i) => <Skeleton key={i} h={40} />)}</div>
        ) : data.items.length === 0 ? (
          <Empty icon="shield" title="Belum ada catatan yang cocok" />
        ) : (
          <>
            <div className="table-scroll">
              <table className="table">
                <thead><tr><th>Waktu</th><th>Kejadian</th><th>Akun</th><th>Lokasi</th><th>IP</th><th>Perangkat</th></tr></thead>
                <tbody>
                  {data.items.map((l) => {
                    const ev = events[l.event] ?? { label: l.event, cls: 'badge-neutral' };
                    return (
                      <tr key={l.id}>
                        <td className="num" style={{ whiteSpace: 'nowrap' }}>{formatDateTime(l.createdAt)}</td>
                        <td><span className={'badge ' + ev.cls}>{ev.label}</span>{l.detail && <span className="sub">{l.detail}</span>}</td>
                        <td>{l.email || '–'}</td>
                        <td style={{ whiteSpace: 'nowrap' }}><Icon name="pin" size={13} /> {l.location || 'Mencari…'}</td>
                        <td className="mono" style={{ fontSize: 13 }}>{l.ip}</td>
                        <td>{l.device}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <div className="pager">
              <span>{data.total} catatan · halaman {data.page} dari {totalPages}</span>
              <div style={{ display: 'flex', gap: 6 }}>
                <button className="btn btn-ghost btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button>
                <button className="btn btn-ghost btn-sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Berikutnya</button>
              </div>
            </div>
          </>
        )}
      </div>
    </>
  );
}
