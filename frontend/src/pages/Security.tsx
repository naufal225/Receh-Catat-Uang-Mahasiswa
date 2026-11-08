// Devices signed in to this account, sign-in methods, and password change.
import { useState, type FormEvent } from 'react';
import { useGoogleEnabled } from '../components/GoogleButton';
import { Icon } from '../components/Icon';
import { useAuth } from '../lib/auth';
import { Empty, ErrorNote, Field, PasswordInput, Skeleton, useConfirm, useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { formatDateTime, timeAgo } from '../lib/format';
import { useFetch } from '../lib/hooks';
import type { Session } from '../lib/types';

function deviceIcon(device: string) {
  return /Android|iPhone|iPad/.test(device) ? '📱' : '💻';
}

export default function Security() {
  const toast = useToast();
  const confirm = useConfirm();
  const { data, error, reload } = useFetch<Session[]>('/sessions');
  const others = (data ?? []).filter((s) => !s.current);

  async function revoke(s: Session) {
    const ok = await confirm({ title: 'Keluarkan perangkat ini?', body: <>{s.device} ({s.ip}) akan langsung keluar dari akun Anda.</>, confirmLabel: 'Keluarkan', danger: true });
    if (!ok) return;
    try {
      await api.del(`/sessions/${s.id}`);
      toast('Perangkat dikeluarkan');
      reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }
  async function revokeOthers() {
    const ok = await confirm({ title: 'Keluarkan semua perangkat lain?', body: `${others.length} perangkat lain akan keluar. Perangkat ini tetap masuk.`, confirmLabel: 'Keluarkan semua', danger: true });
    if (!ok) return;
    try {
      const r = await api.del<{ revoked: number }>('/sessions');
      toast(`${r.revoked} perangkat dikeluarkan`);
      reload();
    } catch (e) {
      toast((e as ApiError).message, 'error');
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Perangkat & keamanan</h1>
          <p>Perangkat yang sedang masuk ke akunmu. Ada yang tidak kamu kenali? Keluarkan dan ganti kata sandi.</p>
        </div>
        {others.length > 0 && <button className="btn btn-ghost" onClick={revokeOthers}><Icon name="logout" size={18} /> Keluarkan semua perangkat lain</button>}
      </div>

      {error && !data && <ErrorNote message={error.message} onRetry={reload} />}
      <div className="card" style={{ overflow: 'hidden', marginBottom: 28 }}>
        {!data ? (
          <div style={{ padding: 18, display: 'grid', gap: 14 }}>{[0, 1].map((i) => <Skeleton key={i} h={48} />)}</div>
        ) : data.length === 0 ? (
          <Empty icon="device" title="Tidak ada sesi aktif" />
        ) : (
          <div className="list-card">
            {data.map((s) => (
              <div key={s.id} className="list-item" style={{ flexWrap: 'wrap' }}>
                <div className="empty-art" style={{ width: 46, height: 46, fontSize: 22 }} aria-hidden="true">{deviceIcon(s.device)}</div>
                <div className="grow">
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <strong>{s.device}</strong>
                    {s.current && <span className="badge badge-income"><Icon name="check" size={12} /> Perangkat ini</span>}
                  </div>
                  <div className="tx-meta">
                    <span><Icon name="pin" size={12} /> {s.location || 'Mencari lokasi…'}</span>
                    <span aria-hidden="true">·</span>
                    <span className="num">IP {s.ip}</span>
                  </div>
                  <div className="tx-meta">
                    Masuk {formatDateTime(s.createdAt)} · Aktif {s.current ? 'sekarang' : timeAgo(s.lastSeenAt)}
                  </div>
                </div>
                {!s.current && <button className="btn btn-ghost btn-sm" onClick={() => revoke(s)}>Keluarkan</button>}
              </div>
            ))}
          </div>
        )}
      </div>

      <SignInMethods />
      <ChangePassword onDone={reload} />
    </>
  );
}

/** Which ways this account can sign in, so nobody is surprised by a missing password. */
function SignInMethods() {
  const { user } = useAuth();
  const googleOn = useGoogleEnabled();
  if (!user || user.role === 'admin' || (!user.googleLinked && !googleOn)) return null;
  return (
    <div className="card card-pad" style={{ maxWidth: 520, marginBottom: 20 }}>
      <h2 style={{ fontSize: 18, fontFamily: 'var(--font-sans)', fontWeight: 750, marginBottom: 12 }}>Metode masuk</h2>
      <div className="method-row">
        <span>Email &amp; kata sandi</span>
        {user.hasPassword
          ? <span className="badge badge-income"><Icon name="check" size={12} /> Aktif</span>
          : <span className="badge badge-neutral">Belum dibuat</span>}
      </div>
      <div className="method-row">
        <span>Google <small className="muted">({user.email})</small></span>
        {user.googleLinked
          ? <span className="badge badge-income"><Icon name="check" size={12} /> Tertaut</span>
          : <span className="badge badge-neutral">Tertaut otomatis saat kamu masuk dengan Google</span>}
      </div>
    </div>
  );
}

function ChangePassword({ onDone }: { onDone: () => void }) {
  const { user, refresh } = useAuth();
  // An account created with Google has no password yet: it creates one
  // without being asked for a current password.
  const creating = user ? !user.hasPassword : false;
  const toast = useToast();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (next.length < 8) return setErrors({ newPassword: 'Kata sandi baru minimal 8 karakter.' });
    setSaving(true);
    try {
      await api.post('/auth/password', { currentPassword: creating ? '' : current, newPassword: next });
      toast(creating ? 'Kata sandi dibuat. Sekarang kamu juga bisa masuk dengan email.' : 'Kata sandi diganti. Perangkat lain telah dikeluarkan.');
      setCurrent('');
      setNext('');
      setErrors({});
      onDone();
      await refresh();
    } catch (err) {
      const e2 = err as ApiError;
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="card card-pad form-grid" style={{ maxWidth: 520 }} onSubmit={submit} noValidate>
      <div>
        <h2 style={{ fontSize: 18, fontFamily: 'var(--font-sans)', fontWeight: 750 }}>{creating ? 'Buat kata sandi' : 'Ganti kata sandi'}</h2>
        <p className="muted" style={{ fontSize: 14 }}>
          {creating
            ? 'Akunmu dibuat lewat Google. Buat kata sandi supaya bisa masuk juga dengan email.'
            : 'Semua perangkat lain akan otomatis dikeluarkan.'}
        </p>
      </div>
      {!creating && (
        <Field label="Kata sandi saat ini" htmlFor="pw-current" error={errors.currentPassword}>
          <PasswordInput id="pw-current" value={current} onChange={setCurrent} autoComplete="current-password" invalid={!!errors.currentPassword} />
        </Field>
      )}
      <Field label="Kata sandi baru" htmlFor="pw-new" error={errors.newPassword} hint="Minimal 8 karakter.">
        <PasswordInput id="pw-new" value={next} onChange={setNext} autoComplete="new-password" invalid={!!errors.newPassword} />
      </Field>
      {errors.form && <div className="alert alert-danger">{errors.form}</div>}
      <div><button type="submit" className="btn btn-primary" disabled={saving}>{saving ? 'Menyimpan…' : creating ? 'Buat kata sandi' : 'Ganti kata sandi'}</button></div>
    </form>
  );
}
