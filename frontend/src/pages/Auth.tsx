// Login and registration. Registration needs only name, email and password:
// no OTP, no email verification, as specified.
import { useState, type FormEvent, type ReactNode } from 'react';
import { flushSync } from 'react-dom';
import { GoogleButton, oauthMessage, OrDivider, takeServerNotice, useGoogleEnabled } from '../components/GoogleButton';
import { Logo } from '../components/Icon';
import { Field, Link, PasswordInput } from '../components/ui';
import { api, ApiError } from '../lib/api';
import { useAuth } from '../lib/auth';
import { navigate, useLocation } from '../lib/router';
import type { User } from '../lib/types';

function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="auth">
      <div className="auth-art">
        <Link to="/" className="brand" style={{ color: '#fffdf8', padding: 0 }}><Logo /> Receh</Link>
        <div>
          <h2>Setiap rupiah <em>kelihatan</em>, dari BCA sampai GoPay.</h2>
          <p style={{ color: '#c9d4cf', marginTop: 14, maxWidth: 420 }}>Catat dalam hitungan detik, lihat ke mana uang pergi, dan biarkan tagihan rutin tercatat sendiri.</p>
        </div>
        <div className="receipt" style={{ maxWidth: 360, color: 'var(--ink)' }}>
          <div className="receipt-list">
            <SampleRow name="Kiriman bulanan" meta="Uang Saku · BCA" amount="+ Rp 1.500.000" income />
            <SampleRow name="Nasi padang" meta="Makan · GoPay" amount="− Rp 18.000" />
            <SampleRow name="Netflix" meta="Langganan · BCA · Rutin" amount="− Rp 54.000" />
          </div>
        </div>
      </div>
      <div className="auth-form">{children}</div>
    </div>
  );
}

function SampleRow({ name, meta, amount, income }: { name: string; meta: string; amount: string; income?: boolean }) {
  return (
    <div className="tx-row">
      <div className={'tx-icon ' + (income ? 'swatch-c3' : 'swatch-c2')}>{name[0]}</div>
      <div className="tx-main"><div className="tx-name">{name}</div><div className="tx-meta">{meta}</div></div>
      <div className={'tx-amount ' + (income ? 'income-text' : 'expense-text')}>{amount}</div>
    </div>
  );
}

function landingFor(u: User) {
  return u.role === 'admin' ? '/admin' : '/app';
}

export function Login() {
  const { setUser } = useAuth();
  const google = useGoogleEnabled();
  const { query } = useLocation();
  // Read once: the server's notice cookie is deleted as soon as it is read.
  const [oauth] = useState(() => {
    const code = query.get('oauth');
    return oauthMessage(code, code === 'ACCOUNT_SUSPENDED' ? takeServerNotice() : null);
  });
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const u = await api.post<User>('/auth/login', { email, password });
      flushSync(() => {
        setUser(u);
        navigate(landingFor(u), { replace: true });
      });
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout>
      <form onSubmit={submit} className="form-grid reveal" noValidate>
        <div>
          <h1>Masuk</h1>
          <p className="muted">Senang melihatmu lagi.</p>
        </div>
        {oauth && !error && <div className={'alert ' + (oauth.tone === 'info' ? 'alert-info' : 'alert-danger')} role="alert">{oauth.text}</div>}
        {error && <div className="alert alert-danger" role="alert">{error}</div>}
        {google && (
          <>
            <GoogleButton />
            <OrDivider />
          </>
        )}
        <Field label="Email" htmlFor="login-email">
          <input id="login-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required autoFocus />
        </Field>
        <Field label="Kata sandi" htmlFor="login-pw">
          <PasswordInput id="login-pw" value={password} onChange={setPassword} autoComplete="current-password" />
        </Field>
        <button className="btn btn-primary btn-block" disabled={busy}>{busy ? 'Memeriksa…' : 'Masuk'}</button>
        <p className="muted" style={{ textAlign: 'center', fontSize: 14 }}>
          Belum punya akun? <Link to="/daftar" style={{ fontWeight: 700, color: 'var(--ink)' }}>Daftar gratis</Link>
        </p>
      </form>
    </AuthLayout>
  );
}

export function Register() {
  const { setUser } = useAuth();
  const google = useGoogleEnabled();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = 'Nama wajib diisi.';
    if (!/^\S+@\S+\.\S+$/.test(email)) next.email = 'Format email belum benar.';
    if (password.length < 8) next.password = 'Kata sandi minimal 8 karakter.';
    setErrors(next);
    if (Object.keys(next).length) return;
    setBusy(true);
    try {
      const u = await api.post<User>('/auth/register', { name, email, password });
      // One synchronous render for both updates, so the /daftar "already signed
      // in" redirect never sees the new user while still on the old path.
      flushSync(() => {
        setUser(u);
        navigate('/app/dompet?baru=1', { replace: true });
      });
    } catch (err) {
      const e2 = err as ApiError;
      setErrors({ [e2.field || 'form']: e2.message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthLayout>
      <form onSubmit={submit} className="form-grid reveal" noValidate>
        <div>
          <h1>Buat akun</h1>
          <p className="muted">Gratis, tanpa OTP. Langsung bisa dipakai.</p>
        </div>
        {errors.form && <div className="alert alert-danger" role="alert">{errors.form}</div>}
        {google && (
          <>
            <GoogleButton label="Daftar dengan Google" />
            <OrDivider />
          </>
        )}
        <Field label="Nama panggilan" htmlFor="reg-name" error={errors.name}>
          <input id="reg-name" className="input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" autoFocus />
        </Field>
        <Field label="Email" htmlFor="reg-email" error={errors.email}>
          <input id="reg-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
        </Field>
        <Field label="Kata sandi" htmlFor="reg-pw" error={errors.password} hint="Minimal 8 karakter.">
          <PasswordInput id="reg-pw" value={password} onChange={setPassword} autoComplete="new-password" invalid={!!errors.password} />
        </Field>
        <button className="btn btn-primary btn-block" disabled={busy}>{busy ? 'Membuat akun…' : 'Daftar & mulai'}</button>
        <p className="muted" style={{ textAlign: 'center', fontSize: 14 }}>
          Sudah punya akun? <Link to="/masuk" style={{ fontWeight: 700, color: 'var(--ink)' }}>Masuk</Link>
        </p>
      </form>
    </AuthLayout>
  );
}
