import { useState, type FormEvent } from 'react';
import { Icon } from '../components/Icon';
import { Empty, ErrorNote, Field, Modal, PasswordInput, Skeleton, useToast } from '../components/ui';
import { api, ApiError, qs } from '../lib/api';
import { formatDateTime, todayISO } from '../lib/format';
import { useDebounced, useFetch } from '../lib/hooks';
import type { Entitlement, Page, User, UserStatus } from '../lib/types';
import { premiumUntil } from './Subscription';

const statusBadge: Record<UserStatus, { cls: string; label: string }> = {
  active: { cls: 'badge-income', label: 'Aktif' },
  suspended: { cls: 'badge-danger', label: 'Ditangguhkan' },
  frozen: { cls: 'badge-warn', label: 'Dibekukan' },
};

const actionCopy: Record<Exclude<UserStatus, 'active'>, { title: string; body: string; button: string }> = {
  suspended: {
    title: 'Tangguhkan pengguna',
    body: 'Pengguna langsung dikeluarkan dari semua perangkat dan tidak bisa masuk sampai diaktifkan lagi. Alasan akan ditampilkan saat ia mencoba masuk.',
    button: 'Tangguhkan',
  },
  frozen: {
    title: 'Bekukan akun',
    body: 'Pengguna tetap bisa masuk, melihat data, dan mengunduh laporan, tetapi semua perubahan ditolak dan transaksi rutin berhenti.',
    button: 'Bekukan',
  },
};

export default function AdminUsers() {
  const toast = useToast();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const debounced = useDebounced(search);
  const { data, error, loading, reload } = useFetch<Page<User>>(`/admin/users${qs({ search: debounced, status, page, pageSize: 20 })}`);
  const [creating, setCreating] = useState(false);
  const [action, setAction] = useState<{ user: User; status: Exclude<UserStatus, 'active'> } | null>(null);
  const [premiumFor, setPremiumFor] = useState<User | null>(null);

  async function activate(u: User) {
    try {
      await api.post(`/admin/users/${u.id}/status`, { status: 'active' });
      toast(`${u.name} aktif kembali`);
      reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.pageSize)) : 1;

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Pengguna</h1>
          <p>Tambahkan, tangguhkan, atau bekukan akun pengguna.</p>
        </div>
        <button className="btn btn-primary" onClick={() => setCreating(true)}><Icon name="plus" size={18} /> Tambah pengguna</button>
      </div>

      <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', marginBottom: 16 }}>
        <input className="input" style={{ maxWidth: 340 }} placeholder="Cari nama atau email…" value={search} onChange={(e) => { setSearch(e.target.value); setPage(1); }} aria-label="Cari pengguna" />
        <div className="segmented" role="group" aria-label="Filter status">
          {[['', 'Semua'], ['active', 'Aktif'], ['suspended', 'Ditangguhkan'], ['frozen', 'Dibekukan']].map(([v, l]) => (
            <button key={v} type="button" aria-pressed={status === v} onClick={() => { setStatus(v); setPage(1); }}>{l}</button>
          ))}
        </div>
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}
      <div className={'card' + (loading && data ? ' dim' : '')} style={{ overflow: 'hidden' }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 12 }}>{[0, 1, 2].map((i) => <Skeleton key={i} h={42} />)}</div>
        ) : data.items.length === 0 ? (
          <Empty icon="users" title="Tidak ada pengguna yang cocok" />
        ) : (
          <>
            <div className="table-scroll">
              <table className="table">
                <thead><tr><th>Pengguna</th><th>Peran</th><th>Paket</th><th>Status</th><th>Terdaftar</th><th style={{ textAlign: 'right' }}>Aksi</th></tr></thead>
                <tbody>
                  {data.items.map((u) => (
                    <tr key={u.id}>
                      <td><strong>{u.name}</strong><span className="sub">{u.email}</span></td>
                      <td>{u.role === 'admin' ? <span className="badge badge-accent">Admin</span> : <span className="badge badge-neutral">Pengguna</span>}</td>
                      <td>
                        {u.role !== 'user' ? <span className="muted">—</span> : (
                          <button className="plan-chip" onClick={() => setPremiumFor(u)} aria-label={`Atur Premium ${u.name}`}>
                            {u.plan === 'premium'
                              ? <span className="badge badge-accent"><Icon name="sparkle" size={12} /> Premium</span>
                              : <span className="badge badge-neutral">Gratis</span>}
                          </button>
                        )}
                      </td>
                      <td>
                        <span className={'badge ' + statusBadge[u.status].cls}>{statusBadge[u.status].label}</span>
                        {u.statusReason && <span className="sub">{u.statusReason}</span>}
                      </td>
                      <td className="num" style={{ whiteSpace: 'nowrap' }}>{formatDateTime(u.createdAt)}</td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                        {u.role === 'admin' ? <span className="muted" style={{ fontSize: 13 }}>Dilindungi</span> : (
                          <div style={{ display: 'inline-flex', gap: 6 }}>
                            <button className="btn btn-ghost btn-sm" onClick={() => setPremiumFor(u)}><Icon name="sparkle" size={16} /> Premium</button>
                            {u.status !== 'active' && <button className="btn btn-ghost btn-sm" onClick={() => activate(u)}><Icon name="check" size={16} /> Aktifkan</button>}
                            {u.status !== 'frozen' && <button className="btn btn-ghost btn-sm" onClick={() => setAction({ user: u, status: 'frozen' })}><Icon name="snow" size={16} /> Bekukan</button>}
                            {u.status !== 'suspended' && <button className="btn btn-ghost btn-sm" style={{ color: 'var(--danger)' }} onClick={() => setAction({ user: u, status: 'suspended' })}><Icon name="ban" size={16} /> Tangguhkan</button>}
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="pager">
              <span>{data.total} pengguna</span>
              <div style={{ display: 'flex', gap: 6 }}>
                <button className="btn btn-ghost btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button>
                <button className="btn btn-ghost btn-sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Berikutnya</button>
              </div>
            </div>
          </>
        )}
      </div>

      <Modal open={creating} onClose={() => setCreating(false)} title="Tambah pengguna">
        {creating && <CreateUserForm onDone={() => { setCreating(false); reload(); }} />}
      </Modal>
      <Modal open={!!action} onClose={() => setAction(null)} title={action ? actionCopy[action.status].title : ''}>
        {action && <StatusForm user={action.user} status={action.status} onDone={() => { setAction(null); reload(); }} />}
      </Modal>
      <Modal open={!!premiumFor} onClose={() => { setPremiumFor(null); reload(); }} title="Premium gratis">
        {premiumFor && <PremiumPanel user={premiumFor} />}
      </Modal>
    </>
  );
}

const sourceText: Record<Entitlement['source'], string> = {
  comp: 'Hadiah admin',
  code: 'Kode undangan',
  paid: 'Berbayar',
};

/** Give a user premium for free (e.g. family), see their history, revoke. */
function PremiumPanel({ user }: { user: User }) {
  const toast = useToast();
  const { data, reload } = useFetch<{ billingEnabled: boolean; items: Entitlement[] }>(`/admin/users/${user.id}/premium`);
  const [mode, setMode] = useState<'forever' | 'until'>('forever');
  const [until, setUntil] = useState('');
  const [note, setNote] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const now = new Date();
  const active = (data?.items ?? []).filter((e) => !e.revokedAt && (!e.endsAt || new Date(e.endsAt) > now));

  async function grant(e: FormEvent) {
    e.preventDefault();
    if (mode === 'until' && !until) return setErrors({ until: 'Pilih tanggal berakhir.' });
    setBusy(true);
    try {
      await api.post(`/admin/users/${user.id}/premium`, { until: mode === 'until' ? until : '', note });
      toast(`${user.name} sekarang Premium`);
      setNote('');
      setErrors({});
      reload();
    } catch (err) {
      const e2 = err as ApiError;
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setBusy(false);
    }
  }

  async function revoke(ent: Entitlement) {
    try {
      await api.del(`/admin/premium/${ent.id}`);
      toast('Premium dicabut');
      reload();
    } catch (err) {
      toast((err as ApiError).message, 'error');
    }
  }

  return (
    <div className="form-grid">
      <p style={{ color: 'var(--ink-2)' }}>
        <strong>{user.name}</strong> ({user.email}) {active.length ? 'sedang Premium.' : 'memakai paket Gratis.'}
      </p>
      {data && !data.billingEnabled && (
        <div className="alert alert-info">Edisi pribadi: semua akun sudah Premium. Pemberian di bawah baru berlaku bila <code>BILLING_ENABLED=true</code>.</div>
      )}

      {(data?.items.length ?? 0) > 0 && (
        <div className="ent-list">
          {data!.items.map((e) => {
            const on = !e.revokedAt && (!e.endsAt || new Date(e.endsAt) > now);
            return (
              <div key={e.id} className="ent-row" style={on ? undefined : { opacity: 0.55 }}>
                <div>
                  <strong>{sourceText[e.source]}{e.promoCode ? ` · ${e.promoCode}` : ''}</strong>
                  <span className="sub">
                    {e.revokedAt ? `Dicabut ${formatDateTime(e.revokedAt)}` : premiumUntil(e.endsAt)}
                    {e.note && ` · ${e.note}`}
                  </span>
                </div>
                {on && <button className="btn btn-ghost btn-sm" style={{ color: 'var(--danger)' }} onClick={() => revoke(e)}>Cabut</button>}
              </div>
            );
          })}
        </div>
      )}

      <form className="form-grid" onSubmit={grant} noValidate>
        <Field label="Berlaku">
          <div className="segmented" role="group" aria-label="Berlaku">
            <button type="button" aria-pressed={mode === 'forever'} onClick={() => setMode('forever')}>Selamanya</button>
            <button type="button" aria-pressed={mode === 'until'} onClick={() => setMode('until')}>Sampai tanggal</button>
          </div>
        </Field>
        {mode === 'until' && (
          <Field label="Sampai tanggal (termasuk)" htmlFor="pm-until" error={errors.until}>
            <input id="pm-until" className="input" type="date" min={todayISO()} value={until} onChange={(e) => setUntil(e.target.value)} />
          </Field>
        )}
        <Field label="Catatan (opsional)" htmlFor="pm-note" error={errors.note} hint="Contoh: Keluarga, adik, beta tester.">
          <input id="pm-note" className="input" value={note} maxLength={200} onChange={(e) => setNote(e.target.value)} />
        </Field>
        {errors.form && <div className="alert alert-danger">{errors.form}</div>}
        <div className="modal-actions"><button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Menyimpan…' : 'Beri Premium gratis'}</button></div>
      </form>
    </div>
  );
}

function StatusForm({ user, status, onDone }: { user: User; status: Exclude<UserStatus, 'active'>; onDone: () => void }) {
  const toast = useToast();
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const copy = actionCopy[status];
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!reason.trim()) return setError('Tulis alasannya. Pengguna akan melihat alasan ini.');
    try {
      await api.post(`/admin/users/${user.id}/status`, { status, reason });
      toast(`${user.name} ${status === 'suspended' ? 'ditangguhkan' : 'dibekukan'}`);
      onDone();
    } catch (err) {
      setError((err as ApiError).message);
    }
  }
  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <p style={{ color: 'var(--ink-2)' }}><strong>{user.name}</strong> ({user.email}). {copy.body}</p>
      <Field label="Alasan" htmlFor="st-reason" error={error}>
        <input id="st-reason" className="input" value={reason} maxLength={200} onChange={(e) => setReason(e.target.value)} placeholder="Contoh: Aktivitas mencurigakan" autoFocus />
      </Field>
      <div className="modal-actions">
        <button type="submit" className={'btn ' + (status === 'suspended' ? 'btn-danger' : 'btn-primary')}>{copy.button}</button>
      </div>
    </form>
  );
}

function CreateUserForm({ onDone }: { onDone: () => void }) {
  const toast = useToast();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});

  function generate() {
    const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    const buf = new Uint32Array(12);
    crypto.getRandomValues(buf);
    setPassword(Array.from(buf, (n) => chars[n % chars.length]).join(''));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.post('/admin/users', { name, email, password });
      toast(`Akun ${email} dibuat. Sampaikan kata sandinya ke pengguna.`);
      onDone();
    } catch (err) {
      const e2 = err as ApiError;
      setErrors({ [e2.field || 'form']: e2.message });
    }
  }
  return (
    <form className="form-grid" onSubmit={submit} noValidate>
      <Field label="Nama" htmlFor="nu-name" error={errors.name}>
        <input id="nu-name" className="input" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </Field>
      <Field label="Email" htmlFor="nu-email" error={errors.email}>
        <input id="nu-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
      </Field>
      <Field label="Kata sandi awal" htmlFor="nu-pw" error={errors.password} hint="Minimal 8 karakter. Catat dulu sebelum menyimpan.">
        <div style={{ display: 'flex', gap: 8 }}>
          <div style={{ flex: 1 }}><PasswordInput id="nu-pw" value={password} onChange={setPassword} autoComplete="new-password" /></div>
          <button type="button" className="btn btn-ghost" onClick={generate}>Buat acak</button>
        </div>
      </Field>
      {password && <div className="alert alert-info mono" style={{ wordBreak: 'break-all' }}>{password}</div>}
      {errors.form && <div className="alert alert-danger">{errors.form}</div>}
      <div className="modal-actions"><button type="submit" className="btn btn-primary">Buat akun</button></div>
    </form>
  );
}
