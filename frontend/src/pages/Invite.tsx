// Opened from an invitation link: /undangan?t=<token>. A visitor who is not
// signed in is sent to sign in or register and brought back here afterwards.
import { useEffect, useState } from 'react';
import { brand } from '../brand';
import { Logo } from '../components/Icon';
import { roleLabel } from '../components/OrgSwitcher';
import { useToast } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useAuth } from '../lib/auth';
import { formatDateTime } from '../lib/format';
import { setNext } from '../lib/next';
import { useOrg } from '../lib/org';
import { navigate, useLocation } from '../lib/router';
import type { OrgInvite, OrgMembership } from '../lib/types';

export default function Invite() {
  const { query } = useLocation();
  const token = query.get('t') ?? '';
  const { user, logout } = useAuth();
  const { refresh, switchTo } = useOrg();
  const toast = useToast();
  const [info, setInfo] = useState<OrgInvite | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const canJoin = user?.role === 'user';

  useEffect(() => {
    if (!token || !canJoin) return;
    setError('');
    api.get<OrgInvite>('/invitations/' + encodeURIComponent(token)).then(setInfo).catch((e: ApiError) => setError(e.message));
  }, [token, canJoin]);

  function signIn(to: '/masuk' | '/daftar') {
    setNext(`/undangan?t=${encodeURIComponent(token)}`);
    navigate(to);
  }

  async function accept() {
    setBusy(true);
    try {
      const o = await api.post<OrgMembership>('/invitations/' + encodeURIComponent(token) + '/accept');
      await refresh();
      switchTo(o.id);
      toast(`Kamu bergabung ke “${o.name}”`);
      navigate('/app/tim', { replace: true });
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  async function switchAccount() {
    setNext(`/undangan?t=${encodeURIComponent(token)}`);
    await logout();
    navigate('/masuk', { replace: true });
  }

  return (
    <div style={{ minHeight: '100dvh', display: 'grid', placeItems: 'center', padding: 20 }}>
      <div className="card card-pad" style={{ width: 'min(440px, 100%)', display: 'grid', gap: 14 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Logo size={30} /> <strong style={{ fontSize: 18 }}>{brand.name}</strong></div>
        {!token ? (
          <p>Link undangan ini tidak lengkap. Minta pengundang mengirim ulang.</p>
        ) : !user ? (
          <>
            <h1 style={{ fontSize: 22 }}>Kamu diundang bergabung ke sebuah tim</h1>
            <p className="muted">Masuk atau daftar dengan email yang diundang untuk melihat dan menerima undangan.</p>
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <button type="button" className="btn btn-primary" onClick={() => signIn('/masuk')}>Masuk</button>
              <button type="button" className="btn btn-ghost" onClick={() => signIn('/daftar')}>Daftar</button>
            </div>
          </>
        ) : !canJoin ? (
          <p>Akun admin tidak bisa bergabung ke tim. Masuk dengan akun pengguna.</p>
        ) : info ? (
          <>
            <h1 style={{ fontSize: 22 }}>Gabung ke “{info.orgName}”?</h1>
            <p className="muted">
              {info.invitedBy ? <>{info.invitedBy} mengundangmu</> : 'Kamu diundang'} sebagai <strong>{roleLabel[info.role]}</strong>. Berlaku sampai {formatDateTime(info.expiresAt)}.
            </p>
            {error && <div className="alert alert-danger" role="alert">{error}</div>}
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <button type="button" className="btn btn-primary" onClick={accept} disabled={busy}>{busy ? 'Bergabung…' : 'Gabung'}</button>
              <button type="button" className="btn btn-ghost" onClick={() => navigate('/app')}>Nanti saja</button>
            </div>
          </>
        ) : error ? (
          <>
            <h1 style={{ fontSize: 22 }}>Undangan tidak bisa dibuka</h1>
            <p className="muted">{error}</p>
            <p className="muted">Undangan hanya bisa dibuka dengan email yang diundang ({user.email} sedang dipakai). Link berlaku 7 hari dan sekali pakai.</p>
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <button type="button" className="btn btn-primary" onClick={switchAccount}>Ganti akun</button>
              <button type="button" className="btn btn-ghost" onClick={() => navigate('/app')}>Ke beranda</button>
            </div>
          </>
        ) : (
          <p className="muted" aria-busy="true">Memeriksa undangan…</p>
        )}
      </div>
    </div>
  );
}
